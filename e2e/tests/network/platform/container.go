package platform

import (
	"context"
	"os"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = DevPodDescribe("container compatibility", func() {
	ginkgo.Context("running container operations", ginkgo.Label("compatibility"), func() {
		var initialDir string

		ginkgo.BeforeEach(func() {
			var err error
			initialDir, err = os.Getwd()
			framework.ExpectNoError(err)
		})

		ginkgo.It("validates network proxy with running container", ginkgo.Label("container-running"), func() {
			ctx := context.Background()
			f := setupDockerProvider(initialDir + "/bin")

			tempDir, err := framework.CopyToTempDir("tests/network/testdata/with-network-proxy")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			out, err := f.DevPodSSH(ctx, tempDir, "echo -n 'running'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "running")

			for range 3 {
				out, err = f.DevPodSSH(ctx, tempDir, "echo -n 'connection'")
				framework.ExpectNoError(err)
				framework.ExpectEqual(strings.TrimSpace(out), "connection")
			}

			out, err = f.DevPodSSH(ctx, tempDir, "echo -n 'stable'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "stable")
		})
	})
})
