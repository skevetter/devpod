package network_test

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/skevetter/devpod/pkg/daemon/workspace/network"
	"github.com/stretchr/testify/suite"
)

type FallbackTransportTestSuite struct {
	suite.Suite
}

func (s *FallbackTransportTestSuite) TestFallbackTriesPrimary() {
	primaryCalled := false
	primary := &network.MockTransport{
		DialFunc: func(ctx context.Context, target string) (network.Conn, error) {
			primaryCalled = true
			return &mockConn{}, nil
		},
	}

	fallback := &network.MockTransport{}

	transport := network.NewFallbackTransport(primary, fallback)
	conn, err := transport.Dial(context.Background(), "test")

	s.NoError(err)
	s.NotNil(conn)
	s.True(primaryCalled, "Should try primary first")
}

func (s *FallbackTransportTestSuite) TestFallbackUsesSecondaryOnFailure() {
	primary := &network.MockTransport{
		DialFunc: func(ctx context.Context, target string) (network.Conn, error) {
			return nil, errors.New("primary failed")
		},
	}

	fallbackCalled := false
	fallback := &network.MockTransport{
		DialFunc: func(ctx context.Context, target string) (network.Conn, error) {
			fallbackCalled = true
			return &mockConn{}, nil
		},
	}

	transport := network.NewFallbackTransport(primary, fallback)
	conn, err := transport.Dial(context.Background(), "test")

	s.NoError(err)
	s.NotNil(conn)
	s.True(fallbackCalled, "Should use fallback on primary failure")
}

func TestFallbackTransportTestSuite(t *testing.T) {
	suite.Run(t, new(FallbackTransportTestSuite))
}

// mockConn for testing
type mockConn struct{}

func (m *mockConn) Read(b []byte) (n int, err error)   { return 0, nil }
func (m *mockConn) Write(b []byte) (n int, err error)  { return len(b), nil }
func (m *mockConn) Close() error                       { return nil }
func (m *mockConn) LocalAddr() net.Addr                { return nil }
func (m *mockConn) RemoteAddr() net.Addr               { return nil }
func (m *mockConn) SetDeadline(t time.Time) error      { return nil }
func (m *mockConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockConn) SetWriteDeadline(t time.Time) error { return nil }
