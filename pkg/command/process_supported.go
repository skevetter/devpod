//go:build linux || darwin || unix

package command

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func isRunning(pid string) (bool, error) {
	parsedPid, err := strconv.Atoi(pid)
	if err != nil {
		return false, err
	}

	process, err := os.FindProcess(parsedPid)
	if err != nil {
		return false, err
	}

	err = process.Signal(syscall.Signal(0))
	if err != nil {
		return false, nil
	}

	return true, nil
}

// isExpectedProcess checks whether the process with the given PID has a
// command line containing the expected command name. This prevents treating
// a stale PID (reused by an unrelated process after a container restart)
// as the original background process.
func isExpectedProcess(pid, commandName string) bool {
	cmdline, err := os.ReadFile(
		"/proc/" + pid + "/cmdline",
	) // #nosec G304,G703 -- pid is from our own PID file, not user input
	if err != nil {
		// /proc not available (e.g., macOS) — fall back to assuming it's ours.
		// On macOS, container-restart PID reuse doesn't apply.
		return true
	}
	// /proc/{pid}/cmdline uses null bytes as separators
	args := strings.ReplaceAll(string(cmdline), "\x00", " ")
	exe := filepath.Base(strings.Split(string(cmdline), "\x00")[0])
	exe = strings.TrimSuffix(exe, " (deleted)")
	return strings.Contains(args, commandName) || strings.Contains(exe, commandName)
}

func kill(pid string) error {
	parsedPid, err := strconv.Atoi(pid)
	if err != nil {
		return err
	}

	_ = syscall.Kill(parsedPid, syscall.SIGTERM)
	time.Sleep(2 * time.Second)
	_ = syscall.Kill(parsedPid, syscall.SIGKILL)
	return nil
}
