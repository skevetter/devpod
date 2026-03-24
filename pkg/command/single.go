package command

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gofrs/flock"
)

type CreateCommand func() (*exec.Cmd, error)

// StartWithLockAndLogging starts CreateCommand returned by createCommand but
// does not wait for it to complete.
// It ensures that only a single command named commandName runs at any time and
// does nothing otherwise.
// The PID of the process it starts is recorded in TMPDIR/commandName.pid,
// while the stdout and stderr are redirected to TMPDIR/commandName.streams.
// These output files are not cleaned up.
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
	defer func(fileLock *flock.Flock) {
		_ = fileLock.Unlock()
	}(fileLock)

	running, err := isProcessRunning(pidFile)
	if err != nil {
		return err
	}
	if running {
		return nil
	}

	// create command
	cmd, err := createCommand()
	if err != nil {
		return err
	}

	// pipe streams into file.streams
	err = redirectStreams(cmd, streamsFile)
	if err != nil {
		return err
	}

	// start process
	err = startAndRecordPid(cmd, pidFile)
	if err != nil {
		return err
	}

	return nil
}

func isProcessRunning(pidFile string) (bool, error) {
	// check if marker file is there
	pid, err := os.ReadFile(pidFile) // #nosec G304: not user input
	if err != nil {
		if !os.IsNotExist(err) {
			return false, err
		}
	} else {
		// check if process id exists
		isRunning, err := IsRunning(string(pid))
		if err != nil {
			return false, err
		} else if isRunning {
			return true, nil
		}
	}

	return false, nil
}

func startAndRecordPid(cmd *exec.Cmd, pidFile string) error {
	err := cmd.Start()
	if err != nil {
		return err
	}

	err = writePidToFile(cmd, pidFile)
	if err != nil {
		return err
	}

	// release process resources
	err = cmd.Process.Release()
	if err != nil {
		return err
	}

	return nil
}

func writePidToFile(cmd *exec.Cmd, pidFile string) error {
	// wait until we have a process id
	for cmd.Process.Pid < 0 {
		time.Sleep(time.Millisecond)
	}

	// write pid to file
	return os.WriteFile(pidFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0o600)
}

func redirectStreams(cmd *exec.Cmd, streamsFile string) error {
	f, err := os.Create(streamsFile) // #nosec G304: not user input
	if err != nil {
		return err
	}
	if cmd.Stderr == nil {
		cmd.Stderr = f
	}
	if cmd.Stdout == nil {
		cmd.Stdout = f
	}
	return nil
}
