package server

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/skevetter/log"
	"github.com/skevetter/ssh"
)

func execNonPTY(sess ssh.Session, cmd *exec.Cmd, log log.Logger) (err error) {
	log.Debugf("execute SSH server command: %s", strings.Join(cmd.Args, " "))
	// init pipes
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	// start the command
	err = cmd.Start()
	if err != nil {
		return fmt.Errorf("start command: %w", err)
	}

	go func() {
		defer func() { _ = stdin.Close() }()

		_, err := io.Copy(stdin, sess)
		if err != nil {
			log.Debugf("Error piping stdin: %v", err)
		}
	}()

	waitGroup := sync.WaitGroup{}
	waitGroup.Go(func() {
		_, err := io.Copy(sess, stdout)
		if err != nil {
			log.Debugf("Error piping stdout: %v", err)
		}
	})

	waitGroup.Go(func() {
		_, err := io.Copy(sess.Stderr(), stderr)
		if err != nil {
			log.Debugf("Error piping stderr: %v", err)
		}
	})

	waitGroup.Wait()
	err = cmd.Wait()
	if err != nil {
		return err
	}

	return nil
}

func execPTY(
	sess ssh.Session,
	ptyReq ssh.Pty,
	winCh <-chan ssh.Window,
	cmd *exec.Cmd,
	log log.Logger,
) (err error) {
	log.Debugf("execute SSH server PTY command: %s", strings.Join(cmd.Args, " "))
	cmd.Env = append(cmd.Env, fmt.Sprintf("TERM=%s", ptyReq.Term))
	// Start the PTY with the client's terminal dimensions. pty.Start
	// (without size) creates the PTY with OS defaults (typically 80x24) which
	// causes rendering corruption in TUI programs (e.g. Neovim split buffers).
	//
	// Similar PTY solutions used in other Go projects:
	//   - coder/wsep:           https://github.com/coder/wsep/blob/master/localexec_unix.go#L64
	//   - wavetermdev/waveterm: https://github.com/wavetermdev/waveterm/blob/main/pkg/shellexec/shellexec.go#L168
	//   - jumpserver/koko:      https://github.com/jumpserver/koko/blob/dev/pkg/localcommand/local_command.go#L49
	//   - daytonaio/daytona:
	//       https://github.com/daytonaio/daytona/blob/main/apps/daemon/pkg/toolbox/process/pty/session.go#L61
	f, err := startPTY(cmd, ptyReq.Window.Width, ptyReq.Window.Height)
	if err != nil {
		return fmt.Errorf("start pty: %w", err)
	}
	defer func() { _ = f.Close() }()

	go func() {
		for win := range winCh {
			setWinSize(f, win.Width, win.Height)
		}
	}()

	go func() {
		defer func() { _ = f.Close() }()

		// copy stdin
		_, _ = io.Copy(f, sess)
	}()

	stdoutDoneChan := make(chan struct{})
	go func() {
		defer func() { _ = f.Close() }()
		defer close(stdoutDoneChan)

		// copy stdout
		_, _ = io.Copy(sess, f)
	}()

	err = cmd.Wait()
	if err != nil {
		return err
	}

	select {
	case <-stdoutDoneChan:
	case <-time.After(time.Second):
	}
	return nil
}
