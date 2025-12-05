package commands

import (
	"context"
	"os"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = DevPodDescribe("agent container daemon", func() {
	ginkgo.Context("container daemon", ginkgo.Label("daemon"), func() {
		var initialDir string
		var f *framework.Framework

		ginkgo.BeforeEach(func() {
			var err error
			initialDir, err = os.Getwd()
			framework.ExpectNoError(err)
			f = setupDockerProvider(initialDir + "/bin")
		})

		ginkgo.It("command exists and shows help", ginkgo.Label("daemon-help"), func() {
			ctx := context.Background()

			tempDir, err := framework.CopyToTempDir("tests/commands/testdata/simple-app")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			out, err := f.DevPodSSH(ctx, tempDir, "/usr/local/bin/devpod agent container daemon --help 2>&1")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.Contains(out, "daemon"), true, "command should exist")
			framework.ExpectEqual(strings.Contains(out, "timeout"), true, "should have timeout flag")
		})

		ginkgo.It("daemon command is available", ginkgo.Label("daemon-available"), func() {
			ctx := context.Background()

			tempDir, err := framework.CopyToTempDir("tests/commands/testdata/simple-app")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			out, err := f.DevPodSSH(ctx, tempDir, "which devpod && /usr/local/bin/devpod agent container 2>&1 | grep daemon || echo 'daemon not found'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.Contains(out, "daemon"), true, "daemon subcommand should be available")
		})
	})
})
