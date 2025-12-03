package platform

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = DevPodDescribe("daemon integration", func() {
	ginkgo.Context("network proxy daemon", ginkgo.Label("daemon"), func() {
		var initialDir string

		ginkgo.BeforeEach(func() {
			var err error
			initialDir, err = os.Getwd()
			framework.ExpectNoError(err)
		})

		ginkgo.It("workspace starts successfully without network proxy", ginkgo.Label("daemon-default"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/../../bin")

			_ = f.DevPodProviderDelete(ctx, "docker")
			err := f.DevPodProviderAdd(ctx, "docker")
			framework.ExpectNoError(err)
			err = f.DevPodProviderUse(ctx, "docker")
			framework.ExpectNoError(err)

			testDir := filepath.Join(initialDir, "testdata", "simple-app")
			name := "test-default-proxy"
			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), name)

			err = f.DevPodUp(ctx, testDir, "--id", name)
			framework.ExpectNoError(err)

			// Verify workspace is accessible
			out, err := f.DevPodSSH(ctx, name, "echo 'workspace running'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "workspace running")
		})

		ginkgo.It("workspace starts successfully with network proxy config", ginkgo.Label("daemon-enabled"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/../../bin")

			_ = f.DevPodProviderDelete(ctx, "docker")
			err := f.DevPodProviderAdd(ctx, "docker")
			framework.ExpectNoError(err)
			err = f.DevPodProviderUse(ctx, "docker")
			framework.ExpectNoError(err)

			testDir := filepath.Join(initialDir, "testdata", "with-network-proxy")
			name := "test-enabled-proxy"
			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), name)

			err = f.DevPodUp(ctx, testDir, "--id", name)
			framework.ExpectNoError(err)

			// Verify workspace is accessible
			out, err := f.DevPodSSH(ctx, name, "echo 'workspace running'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "workspace running")
		})
	})
})
