//go:build !windows

package pty_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/skevetter/devpod/pkg/pty"
	"github.com/skevetter/devpod/pkg/pty/ptytest"
	"github.com/skevetter/ssh"
	"github.com/stretchr/testify/suite"
)

// these constants/vars are used by StartSuite

const cmdEcho = "echo"

var argEcho = []string{"test"}

const (
	cmdCount = "sh"
)

var argCount = []string{"-c", `
i=0
while [ $i -ne 1000 ]
do
        i=$(($i+1))
        echo "$i"
done
`}

const cmdSleep = "sleep"

var argSleep = []string{"30"}

type StartOtherSuite struct {
	suite.Suite
}

func TestStartOtherSuite(t *testing.T) {
	suite.Run(t, new(StartOtherSuite))
}

func (s *StartOtherSuite) TestEcho() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	p, ps := ptytest.Start(s.T(), pty.Command("echo", "test"))
	p.ExpectMatchContext(ctx, "test")
	s.Require().NoError(ps.Wait())
	s.Require().NoError(p.Close())
}

func (s *StartOtherSuite) TestKill() {
	p, ps := ptytest.Start(s.T(), pty.Command("sleep", "30"))
	s.NoError(ps.Kill())
	err := ps.Wait()
	var exitErr *exec.ExitError
	s.True(errors.As(err, &exitErr))
	s.NotEqual(0, exitErr.ExitCode())
	s.Require().NoError(p.Close())
}

func (s *StartOtherSuite) TestInterrupt() {
	p, ps := ptytest.Start(s.T(), pty.Command("sleep", "30"))
	s.NoError(ps.Signal(os.Interrupt))
	err := ps.Wait()
	var exitErr *exec.ExitError
	s.True(errors.As(err, &exitErr))
	s.NotEqual(0, exitErr.ExitCode())
	s.Require().NoError(p.Close())
}

func (s *StartOtherSuite) TestSSHTTY() {
	opts := pty.WithPTYOption(pty.WithSSHRequest(ssh.Pty{
		Window: ssh.Window{
			Width:  80,
			Height: 24,
		},
	}))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	s.T().Cleanup(cancel)
	p, ps := ptytest.Start(s.T(), pty.Command(`/bin/sh`, `-c`, `env | grep SSH_TTY`), opts)
	p.ExpectMatchContext(ctx, "SSH_TTY=/dev/")
	s.Require().NoError(ps.Wait())
	s.Require().NoError(p.Close())
}
