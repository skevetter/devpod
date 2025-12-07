package network

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = DevPodDescribe("heartbeat timeout", func() {
	ginkgo.Context("stale connection removal", ginkgo.Ordered, ginkgo.Label("network", "heartbeat"), func() {
		ginkgo.It("maintains connection with regular activity", ginkgo.Label("heartbeat-active"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker", "-o", "DOCKER_PATH=docker")
			_ = f.DevPodProviderUse(ctx, "docker")

			testDir := filepath.Join(initialDir, "tests/network/testdata/with-network-proxy")
			name := "test-heartbeat-active"
			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), name)

			err := f.DevPodUp(ctx, testDir, "--id", name)
			framework.ExpectNoError(err)

			// Make connections every 2 seconds for 10 seconds
			for range 5 {
				out, err := f.DevPodSSH(ctx, name, "echo 'active'")
				framework.ExpectNoError(err)
				framework.ExpectEqual(strings.TrimSpace(out), "active")
				time.Sleep(2 * time.Second)
			}

			out, err := f.DevPodSSH(ctx, name, "echo 'still active'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "still active")
		})

		ginkgo.It("connection survives short idle period", ginkgo.Label("heartbeat-short-idle"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker", "-o", "DOCKER_PATH=docker")
			_ = f.DevPodProviderUse(ctx, "docker")

			testDir := filepath.Join(initialDir, "tests/network/testdata", "simple-app")
			name := "test-heartbeat-short-idle"
			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), name)

			err := f.DevPodUp(ctx, testDir, "--id", name)
			framework.ExpectNoError(err)

			// First connection
			out, err := f.DevPodSSH(ctx, name, "echo 'before'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "before")

			// Wait 10 seconds (well below timeout)
			time.Sleep(10 * time.Second)

			// Should still work
			out, err = f.DevPodSSH(ctx, name, "echo 'after'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "after")
		})

		ginkgo.It("workspace remains accessible after extended idle", ginkgo.Label("heartbeat-extended-idle"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker", "-o", "DOCKER_PATH=docker")
			_ = f.DevPodProviderUse(ctx, "docker")

			tempDir, err := framework.CopyToTempDir(initialDir + "/tests/network/testdata/simple-app")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			name := "test-heartbeat-extended"
			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), name)

			err = f.DevPodUp(ctx, tempDir, "--id", name)
			framework.ExpectNoError(err)

			// First connection
			out, err := f.DevPodSSH(ctx, name, "echo 'initial'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "initial")

			// Wait 30 seconds (extended idle but still below timeout)
			time.Sleep(30 * time.Second)

			// Workspace should still be accessible (new connection)
			out, err = f.DevPodSSH(ctx, name, "echo 'reconnected'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "reconnected")
		})

		ginkgo.It("handles connection after workspace restart", ginkgo.Label("heartbeat-restart"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker", "-o", "DOCKER_PATH=docker")
			_ = f.DevPodProviderUse(ctx, "docker")

			tempDir, err := framework.CopyToTempDir(initialDir + "/tests/network/testdata/simple-app")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			name := "test-heartbeat-restart"
			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), name)

			// Create workspace
			err = f.DevPodUp(ctx, tempDir, "--id", name)
			framework.ExpectNoError(err)

			// First connection
			out, err := f.DevPodSSH(ctx, name, "echo 'before stop'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "before stop")

			// Stop workspace
			err = f.DevPodWorkspaceStop(ctx, name)
			framework.ExpectNoError(err)

			// Wait a bit
			time.Sleep(5 * time.Second)

			// Start workspace again
			err = f.DevPodUp(ctx, tempDir, "--id", name)
			framework.ExpectNoError(err)

			// Should be able to connect
			out, err = f.DevPodSSH(ctx, name, "echo 'after restart'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "after restart")
		})
	})
})
