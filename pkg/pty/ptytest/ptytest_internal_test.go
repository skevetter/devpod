package ptytest

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/suite"
)

type StdbufSuite struct {
	suite.Suite
}

func TestStdbufSuite(t *testing.T) {
	suite.Run(t, new(StdbufSuite))
}

func (s *StdbufSuite) TestWriteAndRead() {
	var got bytes.Buffer

	b := newStdbuf()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, err := io.Copy(&got, b)
		s.NoError(err)
	}()

	_, err := b.Write([]byte("hello "))
	s.Require().NoError(err)
	_, err = b.Write([]byte("world\n"))
	s.Require().NoError(err)
	_, err = b.Write([]byte("bye\n"))
	s.Require().NoError(err)

	s.Require().NoError(b.Close())
	<-done

	s.Equal("hello world\nbye\n", got.String())
}
