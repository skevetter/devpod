package local

import (
	"context"
	"fmt"
	"net"

	"github.com/skevetter/devpod/pkg/ts"
	"tailscale.com/client/tailscale"
	"tailscale.com/tsnet"
)

// TailscaleClient wraps tsnet.Server for client-side usage
type TailscaleClient struct {
	server *tsnet.Server
}

// NewTailscaleClient creates a new Tailscale client
func NewTailscaleClient(hostname, authKey, stateDir string) *TailscaleClient {
	return &TailscaleClient{
		server: ts.NewServer(&ts.ServerConfig{
			Hostname: hostname,
			AuthKey:  authKey,
			Dir:      stateDir,
		}),
	}
}

// Start starts the Tailscale client
func (tc *TailscaleClient) Start() error {
	return tc.server.Start()
}

// Dial creates a connection to a Tailscale peer
func (tc *TailscaleClient) Dial(ctx context.Context, network, addr string) (net.Conn, error) {
	return tc.server.Dial(ctx, network, addr)
}

// LocalClient returns the Tailscale local client for status queries
func (tc *TailscaleClient) LocalClient() (*tailscale.LocalClient, error) {
	return tc.server.LocalClient()
}

// FindPeer finds a peer by hostname and returns its IP
func (tc *TailscaleClient) FindPeer(ctx context.Context, hostname string) (string, error) {
	lc, err := tc.LocalClient()
	if err != nil {
		return "", fmt.Errorf("get local client: %w", err)
	}

	status, err := lc.Status(ctx)
	if err != nil {
		return "", fmt.Errorf("get status: %w", err)
	}

	for _, peer := range status.Peer {
		if peer.HostName == hostname {
			if len(peer.TailscaleIPs) > 0 {
				return peer.TailscaleIPs[0].String(), nil
			}
		}
	}

	return "", fmt.Errorf("peer not found: %s", hostname)
}

// Close closes the Tailscale client
func (tc *TailscaleClient) Close() error {
	return tc.server.Close()
}
