package commands

import (
	"context"
	"os"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = DevPodDescribe("agent container port-forward", func() {
	ginkgo.Context("port forwarding", ginkgo.Label("port-forward"), func() {
		var initialDir string
		var f *framework.Framework

		ginkgo.BeforeEach(func() {
			var err error
			initialDir, err = os.Getwd()
			framework.ExpectNoError(err)
			f = setupDockerProvider(initialDir + "/bin")
		})

		ginkgo.It("validates required flags", ginkgo.Label("port-forward-flags"), func() {
			ctx := context.Background()

			tempDir, err := framework.CopyToTempDir("tests/commands/testdata/simple-app")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			out, _ := f.DevPodSSH(ctx, tempDir, "/usr/local/bin/devpod agent container port-forward 2>&1 || true")
			framework.ExpectEqual(strings.Contains(out, "required flag"), true, "should require flags")
		})

		ginkgo.It("command exists and is executable", ginkgo.Label("port-forward-exists"), func() {
			ctx := context.Background()

			tempDir, err := framework.CopyToTempDir("tests/commands/testdata/simple-app")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			out, err := f.DevPodSSH(ctx, tempDir, "/usr/local/bin/devpod agent container port-forward --help 2>&1")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.Contains(out, "port-forward"), true, "command should exist")
			framework.ExpectEqual(strings.Contains(out, "local-port"), true, "should have local-port flag")
			framework.ExpectEqual(strings.Contains(out, "remote-addr"), true, "should have remote-addr flag")
		})
	})
})
