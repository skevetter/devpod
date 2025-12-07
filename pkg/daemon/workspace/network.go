package workspace

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/loft-sh/log"
	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/pkg/daemon/workspace/network"
	"github.com/skevetter/devpod/pkg/platform/client"
	"github.com/skevetter/devpod/pkg/ts"
)

// RunNetworkServer starts the workspace network server with all services
func RunNetworkServer(ctx context.Context, d *Daemon, errChan chan<- error, wg *sync.WaitGroup, rootDir string) {
	defer wg.Done()

	if err := os.MkdirAll(rootDir, os.ModePerm); err != nil {
		errChan <- err
		return
	}

	logger := log.NewStdoutLogger(nil, os.Stdout, os.Stderr, logrus.InfoLevel)

	// Create platform client
	config := client.NewConfig()
	config.AccessKey = d.Config.Platform.AccessKey
	config.Host = "https://" + d.Config.Platform.PlatformHost
	config.Insecure = true
	baseClient := client.NewClientFromConfig(config)

	if err := baseClient.RefreshSelf(ctx); err != nil {
		errChan <- fmt.Errorf("failed to refresh client: %w", err)
		return
	}

	// Create workspace server with all services
	workspaceServer := network.NewServer(&network.ServerConfig{
		AccessKey:     d.Config.Platform.AccessKey,
		PlatformHost:  ts.RemoveProtocol(d.Config.Platform.PlatformHost),
		WorkspaceHost: d.Config.Platform.WorkspaceHost,
		Client:        baseClient,
		RootDir:       rootDir,
		LogF: func(format string, args ...any) {
			if logger.GetLevel() == logrus.DebugLevel {
				logger.Debugf(format, args...)
			}
		},
	}, logger)

	if err := workspaceServer.Start(ctx); err != nil {
		errChan <- fmt.Errorf("network server: %w", err)
	}
}
