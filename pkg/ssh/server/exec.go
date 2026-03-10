package server

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/skevetter/devpod/pkg/pty"
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
// Loosely modeled after Coder's startNonPTYSession:
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

	// Start before launching the stdin copier goroutine; if Start fails the
	// goroutine would block forever on a pipe with no reader.
	if err = cmd.Start(); err != nil {
		_ = stdin.Close()
		return fmt.Errorf("start command: %w", err)
	}

	go func() {
		_, _ = io.Copy(stdin, sess)
		_ = stdin.Close()
	}()

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

// ptySession holds the state for a PTY-backed SSH session.
type ptySession struct {
	pc        pty.PTYCmd
	proc      pty.Process
	closeOnce sync.Once
	closeErr  error
}

func (p *ptySession) close() error {
	p.closeOnce.Do(func() { p.closeErr = p.pc.Close() })
	return p.closeErr
}

// handleSignalsAndResize forwards SSH signals and window resize events
// to the PTY process until both channels are closed.
func (p *ptySession) handleSignalsAndResize(
	sigs <-chan ssh.Signal,
	winCh <-chan ssh.Window,
	log log.Logger,
) {
	for sigs != nil || winCh != nil {
		select {
		case sig, ok := <-sigs:
			if !ok {
				sigs = nil
				continue
			}
			p.forwardSignal(sig, log)
		case win, ok := <-winCh:
			if !ok {
				winCh = nil
				continue
			}
			p.resizePTY(win, log)
		}
	}
}

func (p *ptySession) forwardSignal(sig ssh.Signal, log log.Logger) {
	if err := p.proc.Signal(osSignalFrom(sig)); err != nil {
		log.Debugf("failed to signal pty process: %v", err)
	}
}

func (p *ptySession) resizePTY(win ssh.Window, log log.Logger) {
	if err := p.pc.Resize(
		uint16(win.Height), //nolint:gosec // G115: SSH window dimensions fit uint16
		uint16(win.Width),  //nolint:gosec // G115: SSH window dimensions fit uint16
	); err != nil {
		log.Debugf("failed to resize pty: %v", err)
	}
}

// ptyExecParams holds the parameters for a PTY session execution.
type ptyExecParams struct {
	sess   ssh.Session
	ptyReq ssh.Pty
	winCh  <-chan ssh.Window
	cmd    *exec.Cmd
	log    log.Logger
}

// execPTY executes a command with a PTY, handling terminal resize events and
// SSH signal forwarding. Output is copied on the main goroutine to ensure all
// buffered data is flushed before process.Wait().
//
// DisablePTYEmulation prevents double NL→CRNL translation. The kernel's line
// discipline already performs this; the gliderlabs/ssh library's own conversion
// would corrupt terminal escape sequences.
//
// Ported from coder/ssh (Coder's fork of gliderlabs/ssh):
//   - Coder issue:  https://github.com/coder/coder/issues/3371
//   - Neovim issue: https://github.com/neovim/neovim/issues/3875
func execPTY(p ptyExecParams) (retErr error) {
	p.log.Debugf("execute SSH server PTY command: %s", strings.Join(p.cmd.Args, " "))
	p.sess.DisablePTYEmulation()

	// Build an explicit Env so we can inject TERM without losing inherited
	// variables. A nil Env means "inherit parent" in exec.Cmd; appending to
	// nil would create a slice with only TERM, clearing everything else.
	env := p.cmd.Env
	if env != nil {
		env = append(env, fmt.Sprintf("TERM=%s", p.ptyReq.Term))
	} else {
		env = append(os.Environ(), fmt.Sprintf("TERM=%s", p.ptyReq.Term))
	}
	ptycmd := &pty.Cmd{
		Path: p.cmd.Path,
		Args: p.cmd.Args,
		Env:  env,
		Dir:  p.cmd.Dir,
	}

	pc, proc, err := pty.Start(ptycmd, pty.WithPTYOption(pty.WithSSHRequest(p.ptyReq)))
	if err != nil {
		return fmt.Errorf("start pty: %w", err)
	}

	ps := &ptySession{pc: pc, proc: proc}
	defer func() {
		if closeErr := ps.close(); closeErr != nil {
			p.log.Debugf("failed to close pty: %v", closeErr)
			if retErr == nil {
				retErr = closeErr
			}
		}
	}()

	sigs := make(chan ssh.Signal, 1)
	p.sess.Signals(sigs)
	defer func() {
		p.sess.Signals(nil)
		close(sigs)
	}()

	go ps.handleSignalsAndResize(sigs, p.winCh, p.log)

	go func() {
		_, _ = io.Copy(pc.InputWriter(), p.sess)
		_ = ps.close()
	}()

	// Copy output on main goroutine to ensure all data is flushed before Wait.
	_, _ = io.Copy(p.sess, pc.OutputReader())

	return proc.Wait()
}
