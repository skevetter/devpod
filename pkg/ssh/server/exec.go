package server

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/skevetter/log"
	"github.com/skevetter/ssh"
)

func execNonPTY(sess ssh.Session, cmd *exec.Cmd, log log.Logger) (err error) {
	log.WithFields(logrus.Fields{"command": strings.Join(cmd.Args, " ")}).Debug("execute SSH server command")
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

	waitGroup := sync.WaitGroup{}

	waitGroup.Go(func() {
		defer func() { _ = stdin.Close() }()
		_, _ = io.Copy(stdin, sess)
	})

	waitGroup.Go(func() {
		_, _ = io.Copy(sess, stdout)
	})

	waitGroup.Go(func() {
		_, _ = io.Copy(sess.Stderr(), stderr)
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
	log.Debugf("Execute SSH server PTY command: %s", strings.Join(cmd.Args, " "))

	cmd.Env = append(cmd.Env, fmt.Sprintf("TERM=%s", ptyReq.Term))
	f, err := startPTY(cmd)
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
