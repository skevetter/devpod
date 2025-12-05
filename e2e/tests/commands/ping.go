package commands

import (
	"context"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = DevPodDescribe("ping", func() {
	ginkgo.Context("devpod binary", ginkgo.Label("ping"), func() {
		ginkgo.It("devpod binary exists and is executable", ginkgo.Label("cli-binary"), func() {
			ctx := context.Background()
			f := setupDockerProvider(initialDir + "/bin")

			tempDir, err := framework.CopyToTempDir("tests/commands/testdata/simple-app")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			out, err := f.DevPodSSH(ctx, tempDir, "test -x /usr/local/bin/devpod && echo 'executable' || echo 'not executable'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "executable")
		})
	})
})
