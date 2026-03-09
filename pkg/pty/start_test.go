package pty_test

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/skevetter/devpod/pkg/pty"
	"github.com/stretchr/testify/suite"
)

type StartSuite struct {
	suite.Suite
}

func TestStartSuite(t *testing.T) {
	suite.Run(t, new(StartSuite))
}

func (s *StartSuite) TestCopy() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pc, cmd, err := pty.Start(pty.CommandContext(ctx, cmdEcho, argEcho...))
	s.Require().NoError(err)
	b := &bytes.Buffer{}
	readDone := make(chan error, 1)
	go func() {
		_, err := io.Copy(b, pc.OutputReader())
		readDone <- err
	}()

	select {
	case err := <-readDone:
		s.Require().NoError(err)
	case <-ctx.Done():
		s.Fail("read timed out")
	}
	s.Contains(b.String(), "test")

	cmdDone := make(chan error, 1)
	go func() {
		cmdDone <- cmd.Wait()
	}()

	select {
	case err := <-cmdDone:
		s.Require().NoError(err)
	case <-ctx.Done():
		s.Fail("cmd.Wait() timed out")
	}
}

func (s *StartSuite) TestTruncation() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pc, cmd, err := pty.Start(pty.CommandContext(ctx, cmdCount, argCount...))
	s.Require().NoError(err)

	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		buf := &bytes.Buffer{}
		_, _ = io.Copy(buf, pc.OutputReader())
		s.Contains(buf.String(), "1000")
	}()

	cmdDone := make(chan error, 1)
	go func() {
		cmdDone <- cmd.Wait()
	}()

	select {
	case err := <-cmdDone:
		s.Require().NoError(err)
	case <-ctx.Done():
		s.Fail("cmd.Wait() timed out")
	}

	select {
	case <-readDone:
	case <-ctx.Done():
		s.Fail("read timed out")
	}
}

func (s *StartSuite) TestCancelContext() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmdCtx, cmdCancel := context.WithCancel(ctx)

	pc, cmd, err := pty.Start(pty.CommandContext(cmdCtx, cmdSleep, argSleep...))
	s.Require().NoError(err)
	defer func() { _ = pc.Close() }()
	cmdCancel()

	cmdDone := make(chan struct{})
	go func() {
		defer close(cmdDone)
		_ = cmd.Wait()
	}()

	select {
	case <-cmdDone:
	case <-ctx.Done():
		s.Fail("cmd.Wait() timed out")
	}
}
