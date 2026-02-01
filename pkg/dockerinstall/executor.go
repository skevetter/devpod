package dockerinstall

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Executor struct {
	opts *InstallOptions
}

func NewExecutor(opts *InstallOptions) *Executor {
	return &Executor{opts: opts}
}

func (e *Executor) Run(shC, cmdStr string) error {
	if !e.opts.dryRun {
		fprintf(e.opts.stderr, "+ %s\n", cmdStr)
	}

	switch {
	case strings.HasPrefix(shC, "sudo"):
		return e.runCommand(exec.Command("sudo", "-E", "sh", "-c", cmdStr))
	case strings.HasPrefix(shC, "su"):
		return e.runCommand(exec.Command("su", "-c", cmdStr))
	case shC == ShellEcho:
		fprintln(e.opts.stdout, cmdStr)
		return nil
	default:
		return e.runCommand(exec.Command("sh", "-c", cmdStr))
	}
}

func (e *Executor) RunWithRetry(shC, cmdStr string, timeout time.Duration) error {
	start := time.Now()
	attempt := 0

	for {
		attempt++
		fprintln(e.opts.stdout, fmt.Sprintf("running command: %s", cmdStr))

		stderrBuf := &strings.Builder{}
		origStderr := e.opts.stderr
		e.opts.stderr = io.MultiWriter(origStderr, stderrBuf)

		err := e.Run(shC, cmdStr)
		e.opts.stderr = origStderr

		if err == nil {
			fprintln(e.opts.stdout, "command succeeded")
			return nil
		}

		stderrStr := stderrBuf.String()
		isDpkgLock := strings.Contains(stderrStr, "Could not get lock") ||
			strings.Contains(stderrStr, "/var/lib/dpkg/lock")

		if !isDpkgLock {
			return err
		}

		if time.Since(start) >= timeout {
			return fmt.Errorf("timeout waiting for dpkg lock after %v: %w", timeout, err)
		}

		fprintln(origStderr, "waiting for dpkg lock to be released")
		time.Sleep(RetryDelay)
	}
}

func (e *Executor) RunCommands(shC string, cmds []string) error {
	for _, cmd := range cmds {
		if err := e.Run(shC, cmd); err != nil {
			return err
		}
	}
	return nil
}

func (e *Executor) RunCommandsWithRetry(shC string, cmds []string, timeout time.Duration) error {
	for _, cmd := range cmds {
		if err := e.RunWithRetry(shC, cmd, timeout); err != nil {
			return err
		}
	}
	return nil
}

func (e *Executor) runCommand(cmd *exec.Cmd) error {
	cmd.Stdout = e.opts.stdout
	cmd.Stderr = e.opts.stderr
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	return cmd.Run()
}
