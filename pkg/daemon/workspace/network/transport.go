package network

import (
	"context"
	"net"
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
