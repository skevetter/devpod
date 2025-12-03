package local

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/loft-sh/log"
	"github.com/stretchr/testify/suite"
)

type GRPCProxyServerTestSuite struct {
	suite.Suite
	proxy *GRPCProxyServer
}

func TestGRPCProxyServerTestSuite(t *testing.T) {
	suite.Run(t, new(GRPCProxyServerTestSuite))
}

func (s *GRPCProxyServerTestSuite) SetupTest() {
	logger := log.Default.ErrorStreamOnly()
	s.proxy = NewGRPCProxyServer("localhost:50051", logger)
}

func (s *GRPCProxyServerTestSuite) TearDownTest() {
	if s.proxy != nil {
		_ = s.proxy.Stop()
	}
}

func (s *GRPCProxyServerTestSuite) TestNewGRPCProxyServer() {
	logger := log.Default.ErrorStreamOnly()
	proxy := NewGRPCProxyServer("localhost:50051", logger)
	s.NotNil(proxy)
	s.Equal("localhost:50051", proxy.targetAddr)
}

func (s *GRPCProxyServerTestSuite) TestStop() {
	err := s.proxy.Stop()
	s.NoError(err)
}

func (s *GRPCProxyServerTestSuite) TestStartRequiresListener() {
	// Create a listener
	listener, err := net.Listen("tcp", "localhost:0")
	s.NoError(err)
	defer func() { _ = listener.Close() }()

	// Start should not panic
	s.NotPanics(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		go func() { _ = s.proxy.Start(ctx, listener) }()
		time.Sleep(50 * time.Millisecond)
		_ = s.proxy.Stop()
	})
}
