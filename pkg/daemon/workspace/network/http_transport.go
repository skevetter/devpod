package network

import (
	"context"
	"net"
	"time"
)

type HTTPTransport struct {
	host string
	port string
}

func NewHTTPTransport(host, port string) *HTTPTransport {
	return &HTTPTransport{
		host: host,
		port: port,
	}
}

func (h *HTTPTransport) Dial(ctx context.Context, target string) (Conn, error) {
	addr := net.JoinHostPort(h.host, h.port)
	return net.DialTimeout("tcp", addr, 5*time.Second)
}

func (h *HTTPTransport) Close() error {
	return nil
}
