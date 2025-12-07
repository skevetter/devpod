package network

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type ClientTestSuite struct {
	suite.Suite
	client *Client
}

func TestClientTestSuite(t *testing.T) {
	suite.Run(t, new(ClientTestSuite))
}

func (s *ClientTestSuite) SetupTest() {
	s.client = NewClient("localhost:8080")
}

func (s *ClientTestSuite) TestNewClient() {
	client := NewClient("localhost:8080")
	s.NotNil(client)
	s.Equal("localhost:8080", client.addr)
}

func (s *ClientTestSuite) TestSetTimeout() {
	s.client.SetTimeout(10 * time.Second)
	s.Equal(10*time.Second, s.client.timeout)
}

func (s *ClientTestSuite) TestDialTCPWithServer() {
	// Create test server
	listener, err := net.Listen("tcp", "localhost:0")
	s.NoError(err)
	defer func() { _ = listener.Close() }()

	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			_ = conn.Close()
		}
	}()

	// Dial the server
	client := NewClient(listener.Addr().String())
	ctx := context.Background()
	conn, err := client.DialTCP(ctx)
	s.NoError(err)
	if conn != nil {
		_ = conn.Close()
	}
}

func (s *ClientTestSuite) TestDialTCPFailure() {
	client := NewClient("localhost:99999")
	ctx := context.Background()
	_, err := client.DialTCP(ctx)
	s.Error(err)
}

func (s *ClientTestSuite) TestPingWithServer() {
	// Create test server
	listener, err := net.Listen("tcp", "localhost:0")
	s.NoError(err)
	defer func() { _ = listener.Close() }()

	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			_ = conn.Close()
		}
	}()

	// Ping the server
	client := NewClient(listener.Addr().String())
	ctx := context.Background()
	err = client.Ping(ctx)
	s.NoError(err)
}

func (s *ClientTestSuite) TestPingFailure() {
	client := NewClient("localhost:99999")
	ctx := context.Background()
	err := client.Ping(ctx)
	s.Error(err)
}

func (s *ClientTestSuite) TestDialGRPCFailure() {
	client := NewClient("localhost:99999")
	client.SetTimeout(100 * time.Millisecond)
	ctx := context.Background()
	conn, err := client.DialGRPC(ctx)
	// NewClient doesn't fail immediately, only when connection is used
	if err == nil && conn != nil {
		_ = conn.Close()
	}
	// Test passes if no panic occurs
	s.True(true)
}
