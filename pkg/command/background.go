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

// StartBackgroundOnce starts a background process, ensuring only one instance
// with the given commandName runs at a time. If a process is already running
// (determined by PID file), or the lock cannot be acquired, it returns nil.
//
// Process output is redirected to TMPDIR/commandName.streams unless the
// command already has Stdout/Stderr configured. The PID is recorded in
// TMPDIR/commandName.pid. These files are not cleaned up on exit.
func StartBackgroundOnce(commandName string, createCommand CreateCommand) error {
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

	return startCommand(cmd, pidFile, streamsFile)
}

func startCommand(cmd *exec.Cmd, pidFile, streamsFile string) error {
	streamsF, err := openStreamsFile(cmd, streamsFile)
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		closeFile(streamsF)
		return fmt.Errorf("start process: %w", err)
	}
	// Close the parent's copy of the streams fd. After Start() forks, the
	// child has its own copy; the parent no longer needs it.
	closeFile(streamsF)

	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0o600); err != nil {
		// Process is running but untracked. Kill and reap it to prevent orphans/zombies.
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("write pid file (process killed to prevent orphan): %w", err)
	}

	// Release the process handle so the child runs independently of this
	// parent process. After release, we cannot wait for or signal it.
	_ = cmd.Process.Release()

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
	if cmd.Stdout != nil || cmd.Stderr != nil {
		return nil, nil
	}
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

func closeFile(f *os.File) {
	if f != nil {
		_ = f.Close()
	}
}
