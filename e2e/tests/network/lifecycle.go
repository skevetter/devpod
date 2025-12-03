package network

import (
	"context"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = DevPodDescribe("connection lifecycle", func() {
	ginkgo.Context("connection management", ginkgo.Label("network", "lifecycle"), func() {
		ginkgo.It("handles connection open and close", ginkgo.Label("open-close"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker", "-o", "DOCKER_PATH=docker")
			_ = f.DevPodProviderUse(ctx, "docker")

			tempDir, err := framework.CopyToTempDir(initialDir + "/tests/network/testdata/with-network-proxy")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			out, err := f.DevPodSSH(ctx, tempDir, "echo 'opened'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "opened")

			out, err = f.DevPodSSH(ctx, tempDir, "echo 'reopened'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "reopened")
		})

		ginkgo.It("handles rapid connection cycling", ginkgo.Label("rapid-cycling"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker", "-o", "DOCKER_PATH=docker")
			_ = f.DevPodProviderUse(ctx, "docker")

			tempDir, err := framework.CopyToTempDir(initialDir + "/tests/network/testdata/simple-app")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			for range 20 {
				_, err := f.DevPodSSH(ctx, tempDir, "echo 'ping'")
				framework.ExpectNoError(err)
			}
		})

		ginkgo.It("maintains connection after idle period", ginkgo.Label("idle-connection"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker", "-o", "DOCKER_PATH=docker")
			_ = f.DevPodProviderUse(ctx, "docker")

			tempDir, err := framework.CopyToTempDir(initialDir + "/tests/network/testdata/simple-app")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			out, err := f.DevPodSSH(ctx, tempDir, "echo 'before idle'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "before idle")

			time.Sleep(5 * time.Second)

			out, err = f.DevPodSSH(ctx, tempDir, "echo 'after idle'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "after idle")
		})

		ginkgo.It("recovers from command failures", ginkgo.Label("error-recovery"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker", "-o", "DOCKER_PATH=docker")
			_ = f.DevPodProviderUse(ctx, "docker")

			tempDir, err := framework.CopyToTempDir(initialDir + "/tests/network/testdata/simple-app")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			_, _ = f.DevPodSSH(ctx, tempDir, "exit 1")

			out, err := f.DevPodSSH(ctx, tempDir, "echo 'recovered'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "recovered")
		})
	})
})
