package network_test

import (
	"context"
	"testing"

	"github.com/skevetter/devpod/pkg/daemon/workspace/network"
	"github.com/stretchr/testify/suite"
)

type TransportTestSuite struct {
	suite.Suite
}

func (s *TransportTestSuite) TestTransportInterfaceExists() {
	// This will fail until we create the interface
	var _ network.Transport = (*network.MockTransport)(nil)
}

func (s *TransportTestSuite) TestMockTransportDial() {
	called := false
	mock := &network.MockTransport{
		DialFunc: func(ctx context.Context, target string) (network.Conn, error) {
			called = true
			s.Equal("localhost:8080", target)
			return nil, nil
		},
	}

	_, err := mock.Dial(context.Background(), "localhost:8080")
	s.NoError(err)
	s.True(called, "Dial should be called")
}

func TestTransportTestSuite(t *testing.T) {
	suite.Run(t, new(TransportTestSuite))
}
