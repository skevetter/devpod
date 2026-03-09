package ptytest_test

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/skevetter/devpod/pkg/pty/ptytest"
	"github.com/stretchr/testify/suite"
)

type PtytestSuite struct {
	suite.Suite
}

func TestPtytestSuite(t *testing.T) {
	suite.Run(t, new(PtytestSuite))
}

func (s *PtytestSuite) TestEcho() {
	p := ptytest.New(s.T())
	_, err := p.Output().Write([]byte("write"))
	s.Require().NoError(err)
	p.ExpectMatch("write")
	p.WriteLine("read")
}

func (s *PtytestSuite) TestReadLine() {
	if runtime.GOOS == "windows" {
		s.T().Skip("ReadLine is glitchy on windows")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	s.T().Cleanup(cancel)
	p := ptytest.New(s.T())

	_, err := p.Output().Write([]byte("line 1\nline 2\nline 3\nline 4\nline 5"))
	s.Require().NoError(err)
	s.Equal("line 1", p.ReadLine(ctx))
	s.Equal("line 2", p.ReadLine(ctx))
	s.Equal("line 3", p.ReadLine(ctx))
	s.Equal("line 4", p.ReadLine(ctx))
	s.Equal("line 5", p.ExpectMatch("5"))
}
