package network

import (
	"context"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = DevPodDescribe("daemon integration", func() {
	ginkgo.Context("workspace daemon operations", ginkgo.Label("network", "daemon"), func() {
		ginkgo.It("workspace starts with default daemon configuration", ginkgo.Label("daemon-default"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker", "-o", "DOCKER_PATH=docker")
			_ = f.DevPodProviderUse(ctx, "docker")

			tempDir, err := framework.CopyToTempDir(initialDir + "/tests/network/testdata/simple-app")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			// Verify workspace is functional
			out, err := f.DevPodSSH(ctx, tempDir, "echo 'workspace-running'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "workspace-running")
		})

		ginkgo.It("workspace daemon runs successfully", ginkgo.Label("daemon-running"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker", "-o", "DOCKER_PATH=docker")
			_ = f.DevPodProviderUse(ctx, "docker")

			tempDir, err := framework.CopyToTempDir(initialDir + "/tests/network/testdata/simple-app")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			// Verify daemon binary exists and is executable
			out, err := f.DevPodSSH(ctx, tempDir, "which /usr/local/bin/devpod && echo 'daemon-available'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.Contains(out, "daemon-available"), true, "daemon should be available")
		})
	})
})
