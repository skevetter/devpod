package connection

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = DevPodDescribe("connection lifecycle", func() {
	var f *framework.Framework

	ginkgo.Context("connection management", ginkgo.Label("lifecycle"), func() {
		var initialDir string

		ginkgo.BeforeEach(func() {
			var err error
			initialDir, err = os.Getwd()
			framework.ExpectNoError(err)
			f = setupDockerProvider(initialDir + "/bin")
		})

		ginkgo.It("handles connection open and close", ginkgo.Label("open-close"), func() {
			ctx := context.Background()

			tempDir, err := framework.CopyToTempDir("tests/network/testdata/with-network-proxy")
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

			tempDir, err := framework.CopyToTempDir("tests/network/testdata/simple-app")
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

			tempDir, err := framework.CopyToTempDir("tests/network/testdata/simple-app")
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

			tempDir, err := framework.CopyToTempDir("tests/network/testdata/simple-app")
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
