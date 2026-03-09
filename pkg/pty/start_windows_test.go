//go:build windows

package pty_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/skevetter/devpod/pkg/pty"
	"github.com/skevetter/devpod/pkg/pty/ptytest"
	"github.com/stretchr/testify/suite"
)

type StartWindowsSuite struct {
	suite.Suite
}

func TestStartWindowsSuite(t *testing.T) {
	suite.Run(t, new(StartWindowsSuite))
}

func (s *StartWindowsSuite) TestEcho() {
	p, ps := ptytest.Start(s.T(), pty.Command("cmd.exe", "/c", "echo", "test"))
	p.ExpectMatch("test")
	s.Require().NoError(ps.Wait())
	s.Require().NoError(p.Close())
}

func (s *StartWindowsSuite) TestResize() {
	p, _ := ptytest.Start(s.T(), pty.Command("cmd.exe"))
	s.Require().NoError(p.Resize(100, 50))
	s.Require().NoError(p.Close())
}

func (s *StartWindowsSuite) TestKill() {
	p, ps := ptytest.Start(s.T(), pty.Command("cmd.exe"))
	s.NoError(ps.Kill())
	err := ps.Wait()
	var exitErr *exec.ExitError
	s.True(errors.As(err, &exitErr))
	s.NotEqual(0, exitErr.ExitCode())
	s.Require().NoError(p.Close())
}

func (s *StartWindowsSuite) TestInterrupt() {
	p, ps := ptytest.Start(s.T(), pty.Command("cmd.exe"))
	s.NoError(ps.Signal(os.Interrupt))
	err := ps.Wait()
	var exitErr *exec.ExitError
	s.True(errors.As(err, &exitErr))
	s.NotEqual(0, exitErr.ExitCode())
	s.Require().NoError(p.Close())
}

// these constants/vars are used by StartSuite

const cmdEcho = "cmd.exe"

var argEcho = []string{"/c", "echo", "test"}

const (
	countEnd = 1000
	cmdCount = "cmd.exe"
)

var argCount = []string{"/c", fmt.Sprintf("for /L %%n in (1,1,%d) do @echo %%n", countEnd)}

const cmdSleep = "cmd.exe"

var argSleep = []string{"/c", "timeout", "/t", "30"}
