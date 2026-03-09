//go:build !windows

package pty

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"syscall"
)

func startPty(cmdPty *Cmd, opt ...StartOption) (*otherPty, Process, error) {
	var opts startOptions
	for _, o := range opt {
		o(&opts)
	}

	opty, err := newPty(opts.ptyOpts...)
	if err != nil {
		return nil, nil, fmt.Errorf("newPty failed: %w", err)
	}

	origEnv := cmdPty.Env
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
		if runtime.GOOS == "darwin" && strings.Contains(err.Error(), "bad file descriptor") {
			// macOS kernel race: PTY occasionally closes before use. Retry.
			cmdPty.Env = origEnv
			return startPty(cmdPty, opt...)
		}
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
