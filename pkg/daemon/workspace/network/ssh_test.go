package network

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/loft-sh/log"
	"github.com/stretchr/testify/suite"
)

type SSHTunnelTestSuite struct {
	suite.Suite
	tunnel *SSHTunnel
}

func TestSSHTunnelTestSuite(t *testing.T) {
	suite.Run(t, new(SSHTunnelTestSuite))
}

func (s *SSHTunnelTestSuite) SetupTest() {
	logger := log.Default.ErrorStreamOnly()
	s.tunnel = NewSSHTunnel("localhost:0", "localhost:8080", logger)
}

func (s *SSHTunnelTestSuite) TearDownTest() {
	if s.tunnel != nil {
		s.tunnel.Stop()
	}
}

func (s *SSHTunnelTestSuite) TestNewSSHTunnel() {
	logger := log.Default.ErrorStreamOnly()
	tunnel := NewSSHTunnel("localhost:9999", "localhost:8080", logger)
	s.NotNil(tunnel)
	s.Equal("localhost:9999", tunnel.localAddr)
	s.Equal("localhost:8080", tunnel.remoteAddr)
}

func (s *SSHTunnelTestSuite) TestStart() {
	ctx := context.Background()
	err := s.tunnel.Start(ctx)
	s.NoError(err)
	s.NotEmpty(s.tunnel.LocalAddr())
}

func (s *SSHTunnelTestSuite) TestStop() {
	ctx := context.Background()
	err := s.tunnel.Start(ctx)
	s.NoError(err)

	err = s.tunnel.Stop()
	s.NoError(err)
}

func (s *SSHTunnelTestSuite) TestLocalAddr() {
	ctx := context.Background()
	s.tunnel.Start(ctx)
	addr := s.tunnel.LocalAddr()
	s.NotEmpty(addr)
}

func (s *SSHTunnelTestSuite) TestTunnelConnection() {
	// Create a test server
	listener, err := net.Listen("tcp", "localhost:0")
	s.NoError(err)
	defer listener.Close()

	serverAddr := listener.Addr().String()
	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			conn.Write([]byte("tunneled"))
			conn.Close()
		}
	}()

	// Create tunnel to test server
	logger := log.Default.ErrorStreamOnly()
	tunnel := NewSSHTunnel("localhost:0", serverAddr, logger)
	ctx := context.Background()
	err = tunnel.Start(ctx)
	s.NoError(err)
	defer tunnel.Stop()

	// Give it time to start
	time.Sleep(50 * time.Millisecond)

	// Connect through tunnel
	conn, err := net.Dial("tcp", tunnel.LocalAddr())
	if err == nil {
		defer conn.Close()
		buf := make([]byte, 8)
		conn.Read(buf)
		s.Equal("tunneled", string(buf))
	}
}
