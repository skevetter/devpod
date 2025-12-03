package network

import (
	"context"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/skevetter/devpod/e2e/framework"
	"github.com/skevetter/devpod/pkg/daemon/workspace/network"
)

var _ = DevPodDescribe("workspace server integration test suite", func() {
	ginkgo.Context("testing workspace server", ginkgo.Label("network", "workspace-server"), func() {
		ginkgo.It("creates workspace server", func() {
			config := &network.ServerConfig{
				AccessKey:     "test-key",
				PlatformHost:  "platform.test",
				WorkspaceHost: "workspace.test",
				RootDir:       "/tmp/devpod-test",
			}

			server := network.NewServer(config, nil)
			gomega.Expect(server).NotTo(gomega.BeNil())
		})

		ginkgo.It("starts and stops gracefully", func() {
			ginkgo.Skip("Requires Tailscale setup - skipping in CI")

			config := &network.ServerConfig{
				AccessKey:     "test-key",
				PlatformHost:  "platform.test",
				WorkspaceHost: "workspace.test",
				RootDir:       "/tmp/devpod-test",
			}

			server := network.NewServer(config, nil)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// Start in goroutine
			errChan := make(chan error, 1)
			go func() {
				errChan <- server.Start(ctx)
			}()

			// Give it time to start
			time.Sleep(100 * time.Millisecond)

			// Stop
			server.Stop()

			// Wait for start to complete
			select {
			case err := <-errChan:
				// Context cancelled is expected
				if err != nil && err != context.Canceled {
					framework.ExpectNoError(err)
				}
			case <-time.After(2 * time.Second):
				ginkgo.Fail("Server did not stop in time")
			}
		})
	})
})
