package network_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/skevetter/devpod/pkg/daemon/workspace/network"
	"github.com/stretchr/testify/suite"
)

type StdioTransportTestSuite struct {
	suite.Suite
}

func (s *StdioTransportTestSuite) TestNewStdioTransport() {
	stdin := bytes.NewReader([]byte{})
	stdout := &bytes.Buffer{}
	transport := network.NewStdioTransport(stdin, stdout)
	s.NotNil(transport)
}

func (s *StdioTransportTestSuite) TestStdioTransportDial() {
	stdin := bytes.NewReader([]byte("test data"))
	stdout := &bytes.Buffer{}
	transport := network.NewStdioTransport(stdin, stdout)

	conn, err := transport.Dial(context.Background(), "")
	s.NoError(err)
	s.NotNil(conn)
}

func TestStdioTransportTestSuite(t *testing.T) {
	suite.Run(t, new(StdioTransportTestSuite))
}
