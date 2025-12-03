package network

import (
	"context"

	"github.com/loft-sh/log"
	"github.com/skevetter/devpod/pkg/agent/tunnel"
	"github.com/skevetter/devpod/pkg/credentials"
)

// StartCredentialsServer starts the credentials server with the given transport
func StartCredentialsServer(ctx context.Context, port int, client tunnel.TunnelClient, log log.Logger) error {
	// Use existing credentials server implementation
	return credentials.RunCredentialsServer(ctx, port, client, log)
}
