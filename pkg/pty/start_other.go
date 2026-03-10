//go:build !windows

package pty

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"syscall"
)

// maxStartRetries limits the number of retry attempts for the macOS PTY race
// condition workaround. On macOS (darwin), the kernel can occasionally close
// the PTY file descriptor between allocation and process start, causing a
// "bad file descriptor" error from exec. This is a known kernel race that is
// transient, so retrying a small number of times resolves it reliably.
// Env is saved and restored before each retry because appendPtyEnv appends
// to it, and without restoration entries would accumulate across attempts.
const maxStartRetries = 3

func startPty(cmdPty *Cmd, opt ...StartOption) (*otherPty, Process, error) {
	var opts startOptions
	for _, o := range opt {
		o(&opts)
	}

	for attempt := 0; ; attempt++ {
		origEnv := cmdPty.Env
		opty, proc, err := tryStartPty(cmdPty, opts)
		if err == nil {
			return opty, proc, nil
		}
		if attempt < maxStartRetries && runtime.GOOS == "darwin" &&
			strings.Contains(err.Error(), "bad file descriptor") {
			cmdPty.Env = origEnv
			continue
		}
		return nil, nil, err
	}
}

func tryStartPty(cmdPty *Cmd, opts startOptions) (*otherPty, Process, error) {
	opty, err := newPty(opts.ptyOpts...)
	if err != nil {
		return nil, nil, fmt.Errorf("newPty failed: %w", err)
	}

	appendPtyEnv(cmdPty, opty)

	if cmdPty.Context == nil {
		cmdPty.Context = context.Background()
	}
	cmdExec := cmdPty.AsExec()
	cmdExec.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	cmdExec.Stdout = opty.tty
	cmdExec.Stderr = opty.tty
	cmdExec.Stdin = opty.tty

	if err := cmdExec.Start(); err != nil {
		_ = opty.Close()
		return nil, nil, fmt.Errorf("start: %w", err)
	}

	if err := closeTTYAfterStart(opty); err != nil {
		_ = cmdExec.Process.Kill()
		return nil, nil, err
	}

	proc := &otherProcess{pty: opty.pty, cmd: cmdExec, cmdDone: make(chan any)}
	go proc.waitInternal()
	return opty, proc, nil
}

func appendPtyEnv(cmd *Cmd, opty *otherPty) {
	if opty.opts.sshReq != nil {
		cmd.Env = append(cmd.Env, fmt.Sprintf("SSH_TTY=%s", opty.Name()))
	}
	if opty.opts.setGPGTTY {
		cmd.Env = append(cmd.Env, fmt.Sprintf("GPG_TTY=%s", opty.Name()))
	}
}

// closeTTYAfterStart closes the TTY on Linux so the child process holds the
// only reference. On darwin, keeping it open prevents output race conditions.
func closeTTYAfterStart(opty *otherPty) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	if err := opty.tty.Close(); err != nil {
		return fmt.Errorf("close tty: %w", err)
	}
	opty.tty = nil
	return nil
}
