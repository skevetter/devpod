package network_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/skevetter/devpod/pkg/daemon/workspace/network"
	"github.com/stretchr/testify/suite"
)

type CoverageTestSuite struct {
	suite.Suite
}

func (s *CoverageTestSuite) TestHTTPTransportClose() {
	transport := network.NewHTTPTransport("localhost", "8080")
	err := transport.Close()
	s.NoError(err)
}

func (s *CoverageTestSuite) TestStdioTransportClose() {
	stdin := bytes.NewReader([]byte{})
	stdout := &bytes.Buffer{}
	transport := network.NewStdioTransport(stdin, stdout)
	err := transport.Close()
	s.NoError(err)
}

func (s *CoverageTestSuite) TestStdioConnOperations() {
	stdin := bytes.NewReader([]byte("test data"))
	stdout := &bytes.Buffer{}
	transport := network.NewStdioTransport(stdin, stdout)

	conn, err := transport.Dial(context.Background(), "")
	s.NoError(err)

	// Test Read
	buf := make([]byte, 4)
	n, err := conn.Read(buf)
	s.NoError(err)
	s.Equal(4, n)
	s.Equal("test", string(buf))

	// Test Write
	n, err = conn.Write([]byte("response"))
	s.NoError(err)
	s.Equal(8, n)
	s.Equal("response", stdout.String())

	// Test Close
	err = conn.Close()
	s.NoError(err)

	// Test Addr methods
	s.Nil(conn.LocalAddr())
	s.Nil(conn.RemoteAddr())

	// Test deadline methods
	s.NoError(conn.SetDeadline(time.Time{}))
	s.NoError(conn.SetReadDeadline(time.Time{}))
	s.NoError(conn.SetWriteDeadline(time.Time{}))
}

func (s *CoverageTestSuite) TestFallbackTransportClose() {
	primary := &network.MockTransport{}
	fallback := &network.MockTransport{}
	transport := network.NewFallbackTransport(primary, fallback)
	err := transport.Close()
	s.NoError(err)
}

func (s *CoverageTestSuite) TestMockTransportClose() {
	closeCalled := false
	mock := &network.MockTransport{
		CloseFunc: func() error {
			closeCalled = true
			return nil
		},
	}
	err := mock.Close()
	s.NoError(err)
	s.True(closeCalled)
}

func (s *CoverageTestSuite) TestPoolCloseWithActiveConnections() {
	pool := network.NewConnectionPool(5, 10)

	// Add some connections
	conn1 := &testConn{id: "conn1"}
	conn2 := &testConn{id: "conn2"}
	pool.Put(conn1)
	pool.Put(conn2)

	// Close pool
	err := pool.Close()
	s.NoError(err)
}

func (s *CoverageTestSuite) TestCredentialsProxyErrorHandling() {
	// Test with transport that fails
	transport := &network.MockTransport{
		DialFunc: func(ctx context.Context, target string) (network.Conn, error) {
			return nil, context.DeadlineExceeded
		},
	}

	proxy := network.NewCredentialsProxy(transport)
	err := proxy.SendRequest(context.Background(), &network.CredentialRequest{
		Service: "git",
	})

	s.Error(err)
}

func TestCoverageTestSuite(t *testing.T) {
	suite.Run(t, new(CoverageTestSuite))
}
