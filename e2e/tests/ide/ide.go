package ide

import (
	"context"
	"os"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = ginkgo.Describe("devpod ide test suite", ginkgo.Label("ide"), ginkgo.Ordered, func() {
	var initialDir string

	ginkgo.BeforeEach(func() {
		var err error
		initialDir, err = os.Getwd()
		framework.ExpectNoError(err)
	})

	ginkgo.It("start ides", func(ctx context.Context) {
		f := framework.NewDefaultFramework(initialDir + "/bin")
		tempDir, err := framework.CopyToTempDir("tests/ide/testdata")
		framework.ExpectNoError(err)

		err = f.DevPodProviderAdd(ctx, "docker")
		framework.ExpectNoError(err)
		err = f.DevPodProviderUse(ctx, "docker")
		framework.ExpectNoError(err)

		ginkgo.DeferCleanup(func(cleanupCtx context.Context) {
			err = f.DevPodWorkspaceDelete(cleanupCtx, tempDir)
			framework.ExpectNoError(err)
		})

		err = f.DevPodUpWithIDE(ctx, tempDir, "--open-ide=false", "--ide=vscode")
		framework.ExpectNoError(err)

		err = f.DevPodUpWithIDE(ctx, tempDir, "--open-ide=false", "--ide=openvscode")
		framework.ExpectNoError(err)

		err = f.DevPodUpWithIDE(ctx, tempDir, "--open-ide=false", "--ide=jupyternotebook")
		framework.ExpectNoError(err)

		// TODO: Fix broken IDE
		// err = f.DevPodUpWithIDE(ctx, tempDir, "--open-ide=false", "--ide=fleet")
		// framework.ExpectNoError(err)

		// check if ssh works
		err = f.DevPodSSHEchoTestString(ctx, tempDir)
		framework.ExpectNoError(err)

		// TODO: test jetbrains ides
	})
})
