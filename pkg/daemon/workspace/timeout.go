package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/skevetter/devpod/pkg/agent"
)

// SetupActivityFile creates activity file
func SetupActivityFile() error {
	if err := os.MkdirAll(filepath.Dir(agent.ContainerActivityFile), 0755); err != nil {
		return err
	}
	return os.WriteFile(agent.ContainerActivityFile, fmt.Appendf(nil, "%d", time.Now().Unix()), 0644)
}

// RunTimeoutMonitor monitors activity and shuts down on timeout
func RunTimeoutMonitor(ctx context.Context, timeout time.Duration, errChan chan<- error, wg *sync.WaitGroup, log interface{ Debugf(string, ...interface{}) }) {
	defer wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			data, err := os.ReadFile(agent.ContainerActivityFile)
			if err != nil {
				continue
			}

			var lastActivity int64
			if _, err := fmt.Sscanf(string(data), "%d", &lastActivity); err != nil {
				log.Debugf("failed to parse activity timestamp  %v", err)
				continue
			}

			if time.Since(time.Unix(lastActivity, 0)) > timeout {
				errChan <- fmt.Errorf("inactivity timeout")
				return
			}
		}
	}
}
