package network

import (
	"context"
	"errors"
	"net"
	"sync"
)

var ErrPoolExhausted = errors.New("connection pool exhausted")

type ConnectionPool struct {
	maxIdle   int
	maxActive int
	idle      []net.Conn
	active    map[net.Conn]struct{}
	mu        sync.RWMutex
}

func NewConnectionPool(maxIdle, maxActive int) *ConnectionPool {
	return &ConnectionPool{
		maxIdle:   maxIdle,
		maxActive: maxActive,
		idle:      make([]net.Conn, 0, maxIdle),
		active:    make(map[net.Conn]struct{}),
	}
}

func (p *ConnectionPool) Get(ctx context.Context) (net.Conn, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Try to reuse idle connection
	if len(p.idle) > 0 {
		conn := p.idle[len(p.idle)-1]
		p.idle = p.idle[:len(p.idle)-1]
		p.active[conn] = struct{}{}
		return conn, nil
	}

	return nil, ErrPoolExhausted
}

func (p *ConnectionPool) Put(conn net.Conn) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.active, conn)

	if len(p.idle) < p.maxIdle {
		p.idle = append(p.idle, conn)
	} else {
		_ = conn.Close()
	}

	return nil
}

func (p *ConnectionPool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, conn := range p.idle {
		_ = conn.Close()
	}
	p.idle = nil

	for conn := range p.active {
		_ = conn.Close()
	}
	p.active = nil

	return nil
}
