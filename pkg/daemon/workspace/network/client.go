package network

import (
	"context"
	"fmt"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client provides network client helpers
type Client struct {
	addr    string
	timeout time.Duration
}

// NewClient creates a new network client
func NewClient(addr string) *Client {
	return &Client{
		addr:    addr,
		timeout: 30 * time.Second,
	}
}

// DialGRPC dials a gRPC connection
func (c *Client) DialGRPC(ctx context.Context) (*grpc.ClientConn, error) {
	conn, err := grpc.NewClient(c.addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to dial: %w", err)
	}
	return conn, nil
}

// DialTCP dials a TCP connection
func (c *Client) DialTCP(ctx context.Context) (net.Conn, error) {
	var d net.Dialer
	d.Timeout = c.timeout

	conn, err := d.DialContext(ctx, "tcp", c.addr)
	if err != nil {
		return nil, fmt.Errorf("failed to dial: %w", err)
	}
	return conn, nil
}

// Ping checks if the server is reachable
func (c *Client) Ping(ctx context.Context) error {
	conn, err := c.DialTCP(ctx)
	if err != nil {
		return err
	}
	return conn.Close()
}

// SetTimeout sets the connection timeout
func (c *Client) SetTimeout(timeout time.Duration) {
	c.timeout = timeout
}
