package network

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/skevetter/devpod/e2e/framework"
)

// verifyPortListening checks if a port is listening
func verifyPortListening(host string, port string) error {
	address := net.JoinHostPort(host, port)
	conn, err := net.DialTimeout("tcp", address, 5*time.Second)
	if err != nil {
		return fmt.Errorf("port %s not listening: %w", address, err)
	}
	conn.Close()
	return nil
}

// verifyNetworkProxyRunning checks if network proxy is running in workspace
func verifyNetworkProxyRunning(ctx context.Context, f *framework.Framework, workspace string) error {
	out, err := f.DevPodSSH(ctx, workspace, "ps aux | grep -v grep | grep 'network-proxy' || echo 'not found'")
	if err != nil {
		return fmt.Errorf("failed to check network proxy: %w", err)
	}
	if out == "not found" {
		return fmt.Errorf("network proxy not running")
	}
	return nil
}
