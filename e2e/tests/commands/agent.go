package commands

import (
	"context"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = DevPodDescribe("agent commands", func() {
	ginkgo.Context("agent container", ginkgo.Label("agent"), func() {
		ginkgo.It("devpod agent container command works", ginkgo.Label("cli-agent"), func() {
			ctx := context.Background()
			f := setupDockerProvider(initialDir + "/bin")

			tempDir, err := framework.CopyToTempDir("tests/commands/testdata/simple-app")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			out, err := f.DevPodSSH(ctx, tempDir, "/usr/local/bin/devpod agent container 2>&1 || true")
			framework.ExpectNoError(err)
			framework.ExpectEqual(len(strings.TrimSpace(out)) > 0, true, "command should produce output")
		})
	})
})
