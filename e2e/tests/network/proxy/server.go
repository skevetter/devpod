package proxy

import (
	"context"
	"os"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = DevPodDescribe("network proxy server", func() {
	ginkgo.Context("server running", ginkgo.Label("server"), func() {
		var initialDir string

		ginkgo.BeforeEach(func() {
			var err error
			initialDir, err = os.Getwd()
			framework.ExpectNoError(err)
		})

		ginkgo.It("verifies devpod binary exists", ginkgo.Label("server-binary"), func() {
			ctx := context.Background()
			f := setupDockerProvider(initialDir + "/bin")

			tempDir, err := framework.CopyToTempDir("tests/network/testdata/simple-app")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			out, err := f.DevPodSSH(ctx, tempDir, "which /usr/local/bin/devpod")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "/usr/local/bin/devpod")
		})

		ginkgo.It("verifies workspace is functional", ginkgo.Label("server-functional"), func() {
			ctx := context.Background()
			f := setupDockerProvider(initialDir + "/bin")

			tempDir, err := framework.CopyToTempDir("tests/network/testdata/with-network-proxy")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			out, err := f.DevPodSSH(ctx, tempDir, "echo -n 'functional'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "functional")
		})
	})
})
