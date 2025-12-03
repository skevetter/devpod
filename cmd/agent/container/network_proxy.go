package container

import (
	"github.com/loft-sh/log"
	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/pkg/daemon/workspace/network"
	"github.com/spf13/cobra"
)

// NetworkProxyCmd manages the network proxy
type NetworkProxyCmd struct {
	Addr       string
	GRPCTarget string
	HTTPTarget string
}

// NewNetworkProxyCmd creates a new network proxy command
func NewNetworkProxyCmd() *cobra.Command {
	cmd := &NetworkProxyCmd{}
	networkProxyCmd := &cobra.Command{
		Use:   "network-proxy",
		Short: "Start the network proxy server",
		Args:  cobra.NoArgs,
		RunE:  cmd.Run,
	}
	networkProxyCmd.Flags().StringVar(&cmd.Addr, "addr", "localhost:9090", "Address to listen on")
	networkProxyCmd.Flags().StringVar(&cmd.GRPCTarget, "grpc-target", "", "gRPC target address")
	networkProxyCmd.Flags().StringVar(&cmd.HTTPTarget, "http-target", "", "HTTP target address")
	return networkProxyCmd
}

// Run starts the network proxy server
func (cmd *NetworkProxyCmd) Run(c *cobra.Command, args []string) error {
	logger := log.NewStreamLogger(nil, nil, logrus.InfoLevel)

	config := network.ServerConfig{
		Addr:           cmd.Addr,
		GRPCTargetAddr: cmd.GRPCTarget,
		HTTPTargetAddr: cmd.HTTPTarget,
	}

	server := network.NewServer(config, logger)
	logger.Infof("Starting network proxy server on %s", config.Addr)

	ctx := c.Context()
	return server.Start(ctx)
}
