package container

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/loft-sh/log"
	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/pkg/daemon/workspace/network"
	"github.com/spf13/cobra"
)

// SSHTunnelCmd manages SSH tunneling
type SSHTunnelCmd struct {
	LocalAddr  string
	RemoteAddr string
}

// NewSSHTunnelCmd creates a new SSH tunnel command
func NewSSHTunnelCmd() *cobra.Command {
	cmd := &SSHTunnelCmd{}
	sshTunnelCmd := &cobra.Command{
		Use:   "ssh-tunnel",
		Short: "Create an SSH tunnel",
		Args:  cobra.NoArgs,
		RunE:  cmd.Run,
	}
	sshTunnelCmd.Flags().StringVar(&cmd.LocalAddr, "local-addr", "localhost:0", "Local address to bind (use :0 for random port)")
	sshTunnelCmd.Flags().StringVar(&cmd.RemoteAddr, "remote-addr", "", "Remote address (host:port)")
	sshTunnelCmd.MarkFlagRequired("remote-addr")
	return sshTunnelCmd
}

// Run starts SSH tunnel
func (cmd *SSHTunnelCmd) Run(c *cobra.Command, args []string) error {
	logger := log.NewStreamLogger(os.Stdout, os.Stderr, logrus.InfoLevel)
	tunnel := network.NewSSHTunnel(cmd.LocalAddr, cmd.RemoteAddr, logger)

	ctx, cancel := context.WithCancel(c.Context())
	defer cancel()

	if err := tunnel.Start(ctx); err != nil {
		return fmt.Errorf("failed to start tunnel: %w", err)
	}

	logger.Infof("SSH tunnel: %s -> %s", tunnel.LocalAddr(), cmd.RemoteAddr)

	// Wait for interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logger.Info("Stopping SSH tunnel...")
	return tunnel.Stop()
}
