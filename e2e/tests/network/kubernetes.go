package network

import (
	"context"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = DevPodDescribe("kubernetes integration", func() {
	ginkgo.Context("network proxy in kubernetes", ginkgo.Label("network", "kubernetes"), func() {
		ginkgo.It("validates network proxy in kubernetes pod", ginkgo.Label("networkproxy-kubernetes"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/bin")

			tempDir, err := framework.CopyToTempDir(initialDir + "/tests/network/testdata/kubernetes")
			framework.ExpectNoError(err)

			_ = f.DevPodProviderDelete(ctx, "kubernetes")
			err = f.DevPodProviderAdd(ctx, "kubernetes", "-o", "KUBERNETES_NAMESPACE=devpod")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(func() {
				err = f.DevPodProviderDelete(ctx, "kubernetes")
				framework.ExpectNoError(err)
			})

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			out, err := f.DevPodSSH(ctx, tempDir, "echo -n 'kubernetes'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "kubernetes")

			err = f.DevPodWorkspaceDelete(ctx, tempDir)
			framework.ExpectNoError(err)
		})
	})
})
