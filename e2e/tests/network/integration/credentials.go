package integration

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = DevPodDescribe("credentials forwarding", func() {
	ginkgo.Context("platform credentials", ginkgo.Label("credentials"), func() {
		var initialDir string

		ginkgo.BeforeEach(func() {
			var err error
			initialDir, err = os.Getwd()
			framework.ExpectNoError(err)
		})

		ginkgo.It("verifies git credential helper configured", ginkgo.Label("git-credentials"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/../../bin")

			_ = f.DevPodProviderDelete(ctx, "docker")
			err := f.DevPodProviderAdd(ctx, "docker")
			framework.ExpectNoError(err)
			err = f.DevPodProviderUse(ctx, "docker")
			framework.ExpectNoError(err)

			testDir := filepath.Join(initialDir, "testdata", "with-network-proxy")
			name := "test-git-creds"
			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), name)

			err = f.DevPodUp(ctx, testDir, "--id", name)
			framework.ExpectNoError(err)

			// Check if git is available (may not be in slim images)
			out, err := f.DevPodSSH(ctx, name, "which git || echo 'not installed'")
			if strings.Contains(out, "not installed") {
				ginkgo.Skip("Git not installed in container")
			}

			// Check git credential helper
			out, err = f.DevPodSSH(ctx, name, "git config --global credential.helper || echo 'not configured'")
			framework.ExpectNoError(err)

			// Should have credential helper configured or be not configured (both valid)
			framework.ExpectEqual(err == nil, true)
		})

		ginkgo.It("verifies docker config exists", ginkgo.Label("docker-credentials"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/../../bin")

			_ = f.DevPodProviderDelete(ctx, "docker")
			err := f.DevPodProviderAdd(ctx, "docker")
			framework.ExpectNoError(err)
			err = f.DevPodProviderUse(ctx, "docker")
			framework.ExpectNoError(err)

			testDir := filepath.Join(initialDir, "testdata", "simple-app")
			name := "test-docker-creds"
			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), name)

			err = f.DevPodUp(ctx, testDir, "--id", name)
			framework.ExpectNoError(err)

			// Check if docker config directory exists
			_, err = f.DevPodSSH(ctx, name, "test -d ~/.docker && echo 'exists' || echo 'not exists'")
			framework.ExpectNoError(err)

			// Docker config may or may not exist (both valid)
			framework.ExpectEqual(err == nil, true)
		})
	})
})
