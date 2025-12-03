package network

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/loft-sh/log"
)

// PortForwarder handles port forwarding
type PortForwarder struct {
	mu       sync.RWMutex
	forwards map[string]*portForward
	log      log.Logger
}

type portForward struct {
	localPort  string
	remoteAddr string
	listener   net.Listener
	cancel     context.CancelFunc
}

// NewPortForwarder creates a new port forwarder
func NewPortForwarder(log log.Logger) *PortForwarder {
	return &PortForwarder{
		forwards: make(map[string]*portForward),
		log:      log,
	}
}

// Forward starts forwarding a port
func (pf *PortForwarder) Forward(ctx context.Context, localPort, remoteAddr string) error {
	pf.mu.Lock()
	if _, exists := pf.forwards[localPort]; exists {
		pf.mu.Unlock()
		return fmt.Errorf("port %s already forwarded", localPort)
	}
	pf.mu.Unlock()

	listener, err := net.Listen("tcp", "localhost:"+localPort)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	fwd := &portForward{
		localPort:  localPort,
		remoteAddr: remoteAddr,
		listener:   listener,
		cancel:     cancel,
	}

	pf.mu.Lock()
	pf.forwards[localPort] = fwd
	pf.mu.Unlock()

	go pf.acceptConnections(ctx, fwd)
	return nil
}

// Stop stops forwarding a port
func (pf *PortForwarder) Stop(localPort string) error {
	pf.mu.Lock()
	fwd, exists := pf.forwards[localPort]
	if !exists {
		pf.mu.Unlock()
		return fmt.Errorf("port %s not forwarded", localPort)
	}
	delete(pf.forwards, localPort)
	pf.mu.Unlock()

	fwd.cancel()
	return fwd.listener.Close()
}

// List returns all active forwards
func (pf *PortForwarder) List() []string {
	pf.mu.RLock()
	defer pf.mu.RUnlock()
	ports := make([]string, 0, len(pf.forwards))
	for port := range pf.forwards {
		ports = append(ports, port)
	}
	return ports
}

func (pf *PortForwarder) acceptConnections(ctx context.Context, fwd *portForward) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			conn, err := fwd.listener.Accept()
			if err != nil {
				return
			}
			go pf.handleConnection(conn, fwd.remoteAddr)
		}
	}
}

func (pf *PortForwarder) handleConnection(local net.Conn, remoteAddr string) {
	defer local.Close()

	remote, err := net.Dial("tcp", remoteAddr)
	if err != nil {
		pf.log.Errorf("Failed to dial remote: %v", err)
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
