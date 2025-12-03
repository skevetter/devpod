package platform

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = DevPodDescribe("kubernetes integration", func() {
	ginkgo.Context("network proxy in kubernetes", ginkgo.Label("kubernetes"), func() {
		var initialDir string

		ginkgo.BeforeEach(func() {
			var err error
			initialDir, err = os.Getwd()
			framework.ExpectNoError(err)
		})

		ginkgo.It("validates network proxy in kubernetes pod", ginkgo.Label("networkproxy-kubernetes"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/../../bin")

			testDir := filepath.Join(initialDir, "testdata", "kubernetes")

			_ = f.DevPodProviderDelete(ctx, "kubernetes")
			err := f.DevPodProviderAdd(ctx, "kubernetes", "-o", "KUBERNETES_NAMESPACE=devpod")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(func() {
				_ = f.DevPodProviderDelete(ctx, "kubernetes")
			})

			// Create workspace
			err = f.DevPodUp(ctx, testDir)
			framework.ExpectNoError(err)

			// Verify pod is accessible
			out, err := f.DevPodSSH(ctx, testDir, "echo -n 'kubernetes'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "kubernetes")

			// Cleanup
			err = f.DevPodWorkspaceDelete(ctx, testDir)
			framework.ExpectNoError(err)
		})
	})
})
