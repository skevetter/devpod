package network

import (
	"context"
	"net"

	"tailscale.com/client/tailscale"
	"tailscale.com/tsnet"
)

// TailscaleConfig holds Tailscale server configuration
type TailscaleConfig struct {
	Enabled    bool
	Hostname   string
	AuthKey    string
	ControlURL string
	StateDir   string
}

// TailscaleServer wraps tsnet.Server
type TailscaleServer struct {
	config *TailscaleConfig
	server *tsnet.Server
}

// NewTailscaleServer creates a new Tailscale server
func NewTailscaleServer(config *TailscaleConfig) *TailscaleServer {
	return &TailscaleServer{
		config: config,
		server: &tsnet.Server{
			Hostname:   config.Hostname,
			AuthKey:    config.AuthKey,
			ControlURL: config.ControlURL,
			Dir:        config.StateDir,
		},
	}
}

// Start starts the Tailscale server
func (ts *TailscaleServer) Start() error {
	return ts.server.Start()
}

// Listen creates a listener on the Tailscale network
func (ts *TailscaleServer) Listen(network, addr string) (net.Listener, error) {
	return ts.server.Listen(network, addr)
}

// Dial creates a connection on the Tailscale network
func (ts *TailscaleServer) Dial(ctx context.Context, network, addr string) (net.Conn, error) {
	return ts.server.Dial(ctx, network, addr)
}

// LocalClient returns the Tailscale local client
func (ts *TailscaleServer) LocalClient() (*tailscale.LocalClient, error) {
	return ts.server.LocalClient()
}

// Close closes the Tailscale server
func (ts *TailscaleServer) Close() error {
	return ts.server.Close()
}
