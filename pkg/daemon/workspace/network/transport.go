package network

import (
	"context"
	"io"
	"net"
	"time"
)

// Conn wraps net.Conn for testing
type Conn interface {
	net.Conn
}

// Transport abstracts connection mechanism
type Transport interface {
	Dial(ctx context.Context, target string) (Conn, error)
	Close() error
}

// MockTransport for testing
type MockTransport struct {
	DialFunc  func(ctx context.Context, target string) (Conn, error)
	CloseFunc func() error
}

func (m *MockTransport) Dial(ctx context.Context, target string) (Conn, error) {
	if m.DialFunc != nil {
		return m.DialFunc(ctx, target)
	}
	return nil, nil
}

func (m *MockTransport) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

// mockConn for testing
type mockConn struct {
	net.Conn
	id string
}

func (m *mockConn) Read(b []byte) (n int, err error)   { return 0, io.EOF }
func (m *mockConn) Write(b []byte) (n int, err error)  { return len(b), nil }
func (m *mockConn) Close() error                       { return nil }
func (m *mockConn) LocalAddr() net.Addr                { return nil }
func (m *mockConn) RemoteAddr() net.Addr               { return nil }
func (m *mockConn) SetDeadline(t time.Time) error      { return nil }
func (m *mockConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockConn) SetWriteDeadline(t time.Time) error { return nil }
