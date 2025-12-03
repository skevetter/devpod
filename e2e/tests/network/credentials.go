package network

import (
	"context"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = DevPodDescribe("credentials forwarding", func() {
	ginkgo.Context("platform credentials", ginkgo.Label("network", "credentials"), func() {
		ginkgo.It("verifies git credential helper configured", ginkgo.Label("git-credentials"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker", "-o", "DOCKER_PATH=docker")
			_ = f.DevPodProviderUse(ctx, "docker")

			tempDir, err := framework.CopyToTempDir(initialDir + "/tests/network/testdata/with-network-proxy")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			_, err = f.DevPodSSH(ctx, tempDir, "which git || echo 'not installed'")
			framework.ExpectNoError(err)
			_, err = f.DevPodSSH(ctx, tempDir, "git config --global credential.helper || echo 'not configured'")
			framework.ExpectNoError(err)

			framework.ExpectEqual(err == nil, true)
		})

		ginkgo.It("verifies docker config exists", ginkgo.Label("docker-credentials"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker", "-o", "DOCKER_PATH=docker")
			_ = f.DevPodProviderUse(ctx, "docker")

			tempDir, err := framework.CopyToTempDir(initialDir + "/tests/network/testdata/simple-app")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			_, err = f.DevPodSSH(ctx, tempDir, "test -d ~/.docker && echo 'exists' || echo 'not exists'")
			framework.ExpectNoError(err)

			framework.ExpectEqual(err == nil, true)
		})
	})
})
