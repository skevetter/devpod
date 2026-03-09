package server

import (
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/skevetter/log"
	"github.com/skevetter/ssh"
)

// execNonPTY executes a command without a PTY, wiring stdout/stderr directly
// to the SSH session and using a StdinPipe to avoid blocking on stdin.
//
// Process group isolation (SysProcAttr) ensures child processes can be
// properly signaled on shutdown. SSH client signals are forwarded to the
// process.
//
// Modeled after Coder's startNonPTYSession:
//   - https://github.com/coder/coder/blob/main/agent/agentssh/agentssh.go
func execNonPTY(sess ssh.Session, cmd *exec.Cmd, log log.Logger) (err error) {
	log.Debugf("execute SSH server command: %s", strings.Join(cmd.Args, " "))

	cmd.SysProcAttr = cmdSysProcAttr()
	cmd.Stdout = sess
	cmd.Stderr = sess.Stderr()

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	go func() {
		_, _ = io.Copy(stdin, sess)
		_ = stdin.Close()
	}()

	if err = cmd.Start(); err != nil {
		return fmt.Errorf("start command: %w", err)
	}

	sigs := make(chan ssh.Signal, 1)
	sess.Signals(sigs)
	defer func() {
		sess.Signals(nil)
		close(sigs)
	}()
	go func() {
		for sig := range sigs {
			forwardSignal(log, sig, cmd.Process)
		}
	}()

	return cmd.Wait()
}

// execPTY executes a command with a PTY, handling terminal resize events and
// SSH signal forwarding in a combined select loop. Output is copied on the
// main goroutine to ensure all buffered data is flushed before cmd.Wait().
//
// DisablePTYEmulation disables the gliderlabs/ssh minimal PTY emulation
// (NL→CRNL conversion in session.Write). When a real PTY is allocated, the
// kernel's line discipline already performs NL→CRNL translation on output.
// The gliderlabs/ssh library doesn't know this and applies its own conversion,
// resulting in double translation (\n → \r\r\n) that corrupts terminal
// escape sequences.
//
// The fix was ported from coder/ssh (Coder's fork of gliderlabs/ssh):
//   - Coder issue:  https://github.com/coder/coder/issues/3371
//   - Neovim issue: https://github.com/neovim/neovim/issues/3875
//
// Modeled after Coder's startPTYSession:
//   - https://github.com/coder/coder/blob/main/agent/agentssh/agentssh.go
func execPTY(
	sess ssh.Session,
	ptyReq ssh.Pty,
	winCh <-chan ssh.Window,
	cmd *exec.Cmd,
	log log.Logger,
) (retErr error) {
	log.Debugf("execute SSH server PTY command: %s", strings.Join(cmd.Args, " "))
	sess.DisablePTYEmulation()
	cmd.Env = append(cmd.Env, fmt.Sprintf("TERM=%s", ptyReq.Term))

	f, err := startPTY(cmd, ptyReq.Window.Width, ptyReq.Window.Height)
	if err != nil {
		return fmt.Errorf("start pty: %w", err)
	}
	defer func() {
		closeErr := f.Close()
		if closeErr != nil {
			log.Debugf("failed to close pty: %v", closeErr)
			if retErr == nil {
				retErr = closeErr
			}
		}
	}()

	sigs := make(chan ssh.Signal, 1)
	sess.Signals(sigs)
	defer func() {
		sess.Signals(nil)
		close(sigs)
	}()
	go func() {
		for {
			if sigs == nil && winCh == nil {
				return
			}
			select {
			case sig, ok := <-sigs:
				if !ok {
					sigs = nil
					continue
				}
				forwardSignal(log, sig, cmd.Process)
			case win, ok := <-winCh:
				if !ok {
					winCh = nil
					continue
				}
				if err := setWinSize(f, win.Width, win.Height); err != nil {
					log.Debugf("failed to resize pty: %v", err)
				}
			}
		}
	}()

	go func() {
		_, _ = io.Copy(f, sess)
		_ = f.Close()
	}()

	// Copy output on main goroutine to ensure all data is flushed before Wait.
	_, _ = io.Copy(sess, f)

	return cmd.Wait()
}
