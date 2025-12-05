package integration

import (
	"context"
	"os"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = DevPodDescribe("credentials forwarding", func() {
	ginkgo.Context("platform credentials", ginkgo.Label("credentials"), func() {
		var initialDir string
		var f *framework.Framework

		ginkgo.BeforeEach(func() {
			var err error
			initialDir, err = os.Getwd()
			framework.ExpectNoError(err)
			f = setupDockerProvider(initialDir + "/bin")
		})

		ginkgo.It("verifies git credential helper configured", ginkgo.Label("git-credentials"), func() {
			ctx := context.Background()

			tempDir, err := framework.CopyToTempDir("tests/network/testdata/with-network-proxy")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			// Make sure the image git installed for this test
			_, err = f.DevPodSSH(ctx, tempDir, "which git || echo 'not installed'")
			framework.ExpectNoError(err)
			_, err = f.DevPodSSH(ctx, tempDir, "git config --global credential.helper || echo 'not configured'")
			framework.ExpectNoError(err)

			framework.ExpectEqual(err == nil, true)
		})

		ginkgo.It("verifies docker config exists", ginkgo.Label("docker-credentials"), func() {
			ctx := context.Background()

			tempDir, err := framework.CopyToTempDir("tests/network/testdata/simple-app")
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
