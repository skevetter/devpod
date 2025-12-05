package commands

import (
	"context"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = DevPodDescribe("agent container credentials-server", func() {
	ginkgo.Context("credentials server", ginkgo.Label("credentials"), func() {
		ginkgo.It("command exists and shows help", ginkgo.Label("credentials-help"), func() {
			ctx := context.Background()
			f := setupDockerProvider(initialDir + "/bin")

			tempDir, err := framework.CopyToTempDir("tests/commands/testdata/simple-app")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			out, err := f.DevPodSSH(ctx, tempDir, "/usr/local/bin/devpod agent container credentials-server --help 2>&1")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.Contains(out, "credentials-server"), true, "command should exist")
		})

		ginkgo.It("credentials-server command is available", ginkgo.Label("credentials-available"), func() {
			ctx := context.Background()
			f := setupDockerProvider(initialDir + "/bin")

			tempDir, err := framework.CopyToTempDir("tests/commands/testdata/simple-app")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			out, err := f.DevPodSSH(ctx, tempDir, "/usr/local/bin/devpod agent container 2>&1 | grep credentials-server || echo 'not found'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.Contains(out, "credentials-server"), true, "credentials-server should be available")
		})
	})
})
