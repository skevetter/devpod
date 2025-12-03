package platform

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = DevPodDescribe("container compatibility", func() {
	ginkgo.Context("running container operations", ginkgo.Label("compatibility"), func() {
		var initialDir string

		ginkgo.BeforeEach(func() {
			var err error
			initialDir, err = os.Getwd()
			framework.ExpectNoError(err)
		})

		ginkgo.It("validates network proxy with running container", ginkgo.Label("container-running"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/../../bin")

			_ = f.DevPodProviderDelete(ctx, "docker")
			err := f.DevPodProviderAdd(ctx, "docker")
			framework.ExpectNoError(err)
			err = f.DevPodProviderUse(ctx, "docker")
			framework.ExpectNoError(err)

			testDir := filepath.Join(initialDir, "testdata", "with-network-proxy")
			name := "test-container-compat"
			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), name)

			// Create workspace
			err = f.DevPodUp(ctx, testDir, "--id", name)
			framework.ExpectNoError(err)

			// Verify container is running
			out, err := f.DevPodSSH(ctx, name, "echo -n 'running'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "running")

			// Test multiple SSH connections (validates connection tracking)
			for range 3 {
				out, err = f.DevPodSSH(ctx, name, "echo -n 'connection'")
				framework.ExpectNoError(err)
				framework.ExpectEqual(strings.TrimSpace(out), "connection")
			}

			// Verify container still responsive after multiple connections
			out, err = f.DevPodSSH(ctx, name, "echo -n 'stable'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "stable")
		})
	})
})
