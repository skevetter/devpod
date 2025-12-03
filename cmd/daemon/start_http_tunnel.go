package daemon

import (
	"context"
	"fmt"
	"os"

	"github.com/loft-sh/log"
	"github.com/skevetter/devpod/cmd/flags"
	"github.com/skevetter/devpod/pkg/agent/tunnelserver"
	"github.com/skevetter/devpod/pkg/daemon/local"
	"github.com/spf13/cobra"
)

// StartHTTPTunnelCmd holds the cmd flags
type StartHTTPTunnelCmd struct {
	*flags.GlobalFlags

	Port int
}

// NewStartHTTPTunnelCmd creates a new command
func NewStartHTTPTunnelCmd(flags *flags.GlobalFlags) *cobra.Command {
	cmd := &StartHTTPTunnelCmd{
		GlobalFlags: flags,
	}
	startHTTPTunnelCmd := &cobra.Command{
		Use:   "start-http-tunnel",
		Short: "Start HTTP tunnel server for credentials forwarding",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			return cmd.Run(c.Context())
		},
	}
	startHTTPTunnelCmd.Flags().IntVar(&cmd.Port, "port", 8080, "Port to listen on")
	return startHTTPTunnelCmd
}

// Run runs the command logic
func (cmd *StartHTTPTunnelCmd) Run(ctx context.Context) error {
	logger := log.Default.ErrorStreamOnly()

	// Create tunnel client (stdio)
	tunnelClient, err := tunnelserver.NewTunnelClient(os.Stdin, os.Stdout, true, 64)
	if err != nil {
		return fmt.Errorf("error creating tunnel client: %w", err)
	}

	// Create and start HTTP tunnel server
	server := local.NewHTTPTunnelServer(cmd.Port, tunnelClient, logger)
	logger.Infof("HTTP tunnel server listening on port %d", cmd.Port)

	return server.Start(ctx)
}
