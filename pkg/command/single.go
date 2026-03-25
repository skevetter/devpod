package command

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/gofrs/flock"
)

type CreateCommand func() (*exec.Cmd, error)

// StartWithLockAndLogging starts the command produced by createCommand but
// does not wait for it to complete.
// It ensures that only a single command named commandName runs at any time.
// If the lock cannot be acquired or a process is already running (as
// determined by its recorded PID), the function returns nil without starting
// a new process.
// The PID of the process it starts is recorded in TMPDIR/commandName.pid,
// while stdout and stderr are redirected to TMPDIR/commandName.streams if
// they are not already set on the command.
// The .pid, .streams, and .lock files in TMPDIR are not cleaned up.
func StartWithLockAndLogging(commandName string, createCommand CreateCommand) error {
	lockFile := filepath.Join(os.TempDir(), commandName+".lock")
	pidFile := filepath.Join(os.TempDir(), commandName+".pid")
	streamsFile := filepath.Join(os.TempDir(), commandName+".streams")

	// Create a file-based lock to prevent multiple invocations of this function
	// before the process is created.
	fileLock := flock.New(lockFile)
	locked, err := fileLock.TryLock()
	if err != nil {
		return fmt.Errorf("acquire lock: %w", err)
	} else if !locked {
		return nil
	}
	defer func() {
		if unlockErr := fileLock.Unlock(); unlockErr != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to release lock %s: %v\n", lockFile, unlockErr)
		}
	}()

	running, err := isProcessRunning(pidFile)
	if err != nil {
		return err
	}
	if running {
		return nil
	}

	cmd, err := createCommand()
	if err != nil {
		return err
	}

	streamsF, err := openStreamsFile(cmd, streamsFile)
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		streamsF.Close()
		return fmt.Errorf("start process: %w", err)
	}
	// Close the parent's copy of the streams fd. After Start() forks, the
	// child has its own copy; the parent no longer needs it.
	streamsF.Close()

	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0o600); err != nil {
		// Process is running but untracked. Kill it to prevent orphans.
		_ = cmd.Process.Kill()
		return fmt.Errorf("write pid file (process killed to prevent orphan): %w", err)
	}

	return nil
}

func isProcessRunning(pidFile string) (bool, error) {
	pid, err := os.ReadFile(pidFile) // #nosec G304: not user input
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("read pid file %s: %w", pidFile, err)
	}

	isRunning, err := IsRunning(string(pid))
	if err != nil {
		// PID file is corrupt or contains an invalid PID.
		// Treat as "not running" and clean up the stale file.
		_ = os.Remove(pidFile)
		return false, nil
	}

	return isRunning, nil
}

func openStreamsFile(cmd *exec.Cmd, streamsFile string) (*os.File, error) {
	f, err := os.Create(streamsFile) // #nosec G304: not user input
	if err != nil {
		return nil, err
	}
	if cmd.Stderr == nil {
		cmd.Stderr = f
	}
	if cmd.Stdout == nil {
		cmd.Stdout = f
	}
	return f, nil
}
