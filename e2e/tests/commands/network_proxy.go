package commands

import (
	"context"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = DevPodDescribe("agent container network-proxy", func() {
	ginkgo.Context("network proxy server", ginkgo.Label("network-proxy"), func() {
		ginkgo.It("starts network proxy server", ginkgo.Label("network-proxy-start"), func() {
			ctx := context.Background()
			f := setupDockerProvider(initialDir + "/bin")

			tempDir, err := framework.CopyToTempDir("tests/commands/testdata/simple-app")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			out, err := f.DevPodSSH(ctx, tempDir, "/usr/local/bin/devpod agent container network-proxy --help 2>&1")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.Contains(out, "network-proxy"), true, "network-proxy command should exist")
		})

		ginkgo.It("network proxy command has correct flags", ginkgo.Label("network-proxy-flags"), func() {
			ctx := context.Background()
			f := setupDockerProvider(initialDir + "/bin")

			tempDir, err := framework.CopyToTempDir("tests/commands/testdata/simple-app")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			out, err := f.DevPodSSH(ctx, tempDir, "/usr/local/bin/devpod agent container network-proxy --help 2>&1")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.Contains(out, "addr"), true, "should have addr flag")
			framework.ExpectEqual(strings.Contains(out, "grpc-target"), true, "should have grpc-target flag")
			framework.ExpectEqual(strings.Contains(out, "http-target"), true, "should have http-target flag")
		})
	})
})
