package commands

import (
	"context"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = DevPodDescribe("agent container commands", func() {
	ginkgo.Context("agent commands", ginkgo.Label("agent"), func() {
		ginkgo.It("devpod binary exists and is executable", ginkgo.Label("agent-binary"), func() {
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

		ginkgo.It("daemon command exists", ginkgo.Label("daemon"), func() {
			ctx := context.Background()
			f := setupDockerProvider(initialDir + "/bin")

			tempDir, err := framework.CopyToTempDir("tests/commands/testdata/simple-app")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			out, err := f.DevPodSSH(ctx, tempDir, "/usr/local/bin/devpod agent container daemon --help 2>&1")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.Contains(out, "daemon"), true, "daemon command should exist")
			framework.ExpectEqual(strings.Contains(out, "Starts the DevPod network daemon"), true, "should have correct description")
		})

		ginkgo.It("credentials-server command is available", ginkgo.Label("credentials-server"), func() {
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
	})
})
