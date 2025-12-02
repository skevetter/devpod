package up

import (
	"context"
	"os"
	"path/filepath"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = DevPodDescribe("devpod extra devcontainer test suite", func() {
	ginkgo.Context("testing extra devcontainer paths", ginkgo.Label("up", "extra-devcontainer"), ginkgo.Ordered, func() {
		var f *framework.Framework
		var initialDir string

		ginkgo.BeforeEach(func(ctx context.Context) {
			var err error
			initialDir, err = os.Getwd()
			framework.ExpectNoError(err)

			f, err = setupDockerProvider(initialDir+"/bin", "docker")
			framework.ExpectNoError(err)
		})

		ginkgo.Context("with docker provider", ginkgo.Ordered, func() {
			ginkgo.It("should merge extra devcontainer config", func(ctx context.Context) {
				tempDir, err := setupWorkspace("tests/up/testdata/extra-devcontainer", initialDir, f)
				framework.ExpectNoError(err)

				extraConfigPath := filepath.Join(tempDir, "extra.json")
				err = f.DevPodUp(ctx, "--extra-devcontainer-path", extraConfigPath, tempDir)
				framework.ExpectNoError(err)

				// Verify workspace is running
				status, err := f.DevPodStatus(ctx, tempDir)
				framework.ExpectNoError(err)
				framework.ExpectEqual(status.State, "Running")

				// Verify environment variable from extra config is set
				out, err := f.DevPodSSH(ctx, tempDir, "echo -n $EXTRA_VAR")
				framework.ExpectNoError(err)
				framework.ExpectEqual(out, "extra_value")

				// Verify mount from extra config exists
				_, err = f.DevPodSSH(ctx, tempDir, "test -d /workspace/tmp")
				framework.ExpectNoError(err)

				// Cleanup
				err = f.DevPodWorkspaceDelete(ctx, tempDir)
				framework.ExpectNoError(err)
			}, ginkgo.SpecTimeout(framework.GetTimeout()))
		})

		ginkgo.AfterEach(func(ctx context.Context) {
			_ = os.Chdir(initialDir)
			if f != nil {
				_ = f.DevPodProviderDelete(ctx, "docker")
			}
		})
	})
})
