package network

import (
	"context"
	"fmt"
	"io"
	"net"

	"github.com/loft-sh/log"
)

// SSHTunnel handles SSH tunneling
type SSHTunnel struct {
	localAddr  string
	remoteAddr string
	listener   net.Listener
	log        log.Logger
}

// NewSSHTunnel creates a new SSH tunnel
func NewSSHTunnel(localAddr, remoteAddr string, log log.Logger) *SSHTunnel {
	return &SSHTunnel{
		localAddr:  localAddr,
		remoteAddr: remoteAddr,
		log:        log,
	}
}

// Start starts the SSH tunnel
func (t *SSHTunnel) Start(ctx context.Context) error {
	listener, err := net.Listen("tcp", t.localAddr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	t.listener = listener

	go t.acceptConnections(ctx)
	return nil
}

// Stop stops the SSH tunnel
func (t *SSHTunnel) Stop() error {
	if t.listener != nil {
		return t.listener.Close()
	}
	return nil
}

// LocalAddr returns the local address
func (t *SSHTunnel) LocalAddr() string {
	if t.listener != nil {
		return t.listener.Addr().String()
	}
	return t.localAddr
}

func (t *SSHTunnel) acceptConnections(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			conn, err := t.listener.Accept()
			if err != nil {
				return
			}
			go t.handleConnection(conn)
		}
	}
}

func (t *SSHTunnel) handleConnection(local net.Conn) {
	defer local.Close()

	remote, err := net.Dial("tcp", t.remoteAddr)
	if err != nil {
		t.log.Errorf("Failed to dial remote: %v", err)
		return
	}
	defer remote.Close()

	done := make(chan error, 2)
	go func() {
		_, err := io.Copy(remote, local)
		done <- err
	}()
	go func() {
		_, err := io.Copy(local, remote)
		done <- err
	}()

	<-done
}
