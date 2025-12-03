package proxy

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = DevPodDescribe("network proxy server", func() {
	ginkgo.Context("server running", ginkgo.Label("server"), func() {
		var initialDir string

		ginkgo.BeforeEach(func() {
			var err error
			initialDir, err = os.Getwd()
			framework.ExpectNoError(err)
		})

		ginkgo.It("verifies devpod binary exists", ginkgo.Label("server-binary"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/../../bin")

			_ = f.DevPodProviderDelete(ctx, "docker")
			err := f.DevPodProviderAdd(ctx, "docker")
			framework.ExpectNoError(err)
			err = f.DevPodProviderUse(ctx, "docker")
			framework.ExpectNoError(err)

			testDir := filepath.Join(initialDir, "testdata", "simple-app")
			name := "test-server-binary"
			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), name)

			err = f.DevPodUp(ctx, testDir, "--id", name)
			framework.ExpectNoError(err)

			// Check if devpod binary exists
			out, err := f.DevPodSSH(ctx, name, "which /usr/local/bin/devpod")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "/usr/local/bin/devpod")
		})

		ginkgo.It("verifies workspace is functional", ginkgo.Label("server-functional"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/../../bin")

			_ = f.DevPodProviderDelete(ctx, "docker")
			err := f.DevPodProviderAdd(ctx, "docker")
			framework.ExpectNoError(err)
			err = f.DevPodProviderUse(ctx, "docker")
			framework.ExpectNoError(err)

			testDir := filepath.Join(initialDir, "testdata", "with-network-proxy")
			name := "test-server-functional"
			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), name)

			err = f.DevPodUp(ctx, testDir, "--id", name)
			framework.ExpectNoError(err)

			// Verify workspace is functional
			out, err := f.DevPodSSH(ctx, name, "echo -n 'functional'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "functional")
		})
	})
})
