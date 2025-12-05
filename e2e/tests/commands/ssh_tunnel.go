package commands

import (
	"context"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = DevPodDescribe("agent container ssh-tunnel", func() {
	ginkgo.Context("SSH tunneling", ginkgo.Label("ssh-tunnel"), func() {
		ginkgo.It("validates required flags", ginkgo.Label("ssh-tunnel-flags"), func() {
			ctx := context.Background()
			f := setupDockerProvider(initialDir + "/bin")

			tempDir, err := framework.CopyToTempDir("tests/commands/testdata/simple-app")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			out, _ := f.DevPodSSH(ctx, tempDir, "/usr/local/bin/devpod agent container ssh-tunnel 2>&1 || true")
			framework.ExpectEqual(strings.Contains(out, "required flag"), true, "should require remote-addr flag")
		})

		ginkgo.It("command exists and is executable", ginkgo.Label("ssh-tunnel-exists"), func() {
			ctx := context.Background()
			f := setupDockerProvider(initialDir + "/bin")

			tempDir, err := framework.CopyToTempDir("tests/commands/testdata/simple-app")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			out, err := f.DevPodSSH(ctx, tempDir, "/usr/local/bin/devpod agent container ssh-tunnel --help 2>&1")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.Contains(out, "ssh-tunnel"), true, "command should exist")
			framework.ExpectEqual(strings.Contains(out, "local-addr"), true, "should have local-addr flag")
			framework.ExpectEqual(strings.Contains(out, "remote-addr"), true, "should have remote-addr flag")
		})
	})
})
