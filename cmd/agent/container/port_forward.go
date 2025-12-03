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

// PortForwardCmd manages port forwarding
type PortForwardCmd struct {
	LocalPort  string
	RemoteAddr string
}

// NewPortForwardCmd creates a new port forward command
func NewPortForwardCmd() *cobra.Command {
	cmd := &PortForwardCmd{}
	portForwardCmd := &cobra.Command{
		Use:   "port-forward",
		Short: "Forward a local port to a remote address",
		Args:  cobra.NoArgs,
		RunE:  cmd.Run,
	}
	portForwardCmd.Flags().StringVar(&cmd.LocalPort, "local-port", "", "Local port to forward")
	portForwardCmd.Flags().StringVar(&cmd.RemoteAddr, "remote-addr", "", "Remote address (host:port)")
	portForwardCmd.MarkFlagRequired("local-port")
	portForwardCmd.MarkFlagRequired("remote-addr")
	return portForwardCmd
}

// Run starts port forwarding
func (cmd *PortForwardCmd) Run(c *cobra.Command, args []string) error {
	logger := log.NewStreamLogger(os.Stdout, os.Stderr, logrus.InfoLevel)
	forwarder := network.NewPortForwarder(logger)

	ctx, cancel := context.WithCancel(c.Context())
	defer cancel()

	logger.Infof("Forwarding localhost:%s -> %s", cmd.LocalPort, cmd.RemoteAddr)
	if err := forwarder.Forward(ctx, cmd.LocalPort, cmd.RemoteAddr); err != nil {
		return fmt.Errorf("failed to start port forward: %w", err)
	}

	// Wait for interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logger.Info("Stopping port forward...")
	return forwarder.Stop(cmd.LocalPort)
}
