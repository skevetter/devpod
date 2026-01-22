package up

import (
	"context"
	"fmt"
	"os"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = ginkgo.Describe("testing up command for working with git repositories", ginkgo.Label("up-git-repositories"), func() {
	var initialDir string

	ginkgo.BeforeEach(func() {
		var err error
		initialDir, err = os.Getwd()
		framework.ExpectNoError(err)
	})

	ginkgo.It("should allow checkout of a GitRepo from a commit hash", func() {
		ctx := context.Background()
		f, err := setupDockerProvider(initialDir+"/bin", "docker")
		framework.ExpectNoError(err)

		name := "sha256-0c1547c"
		ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), name)

		// Wait for devpod workspace to come online (deadline: 30s)
		err = f.DevPodUp(ctx, "github.com/microsoft/vscode-remote-try-python@sha256:0c1547c")
		framework.ExpectNoError(err)
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("should allow checkout of a GitRepo from a pull request reference", func() {
		ctx := context.Background()
		f, err := setupDockerProvider(initialDir+"/bin", "docker")
		framework.ExpectNoError(err)

		name := "pr100"
		ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), name)

		// Wait for devpod workspace to come online (deadline: 30s)
		err = f.DevPodUp(ctx, "github.com/loft-sh/devpod")
		framework.ExpectNoError(err)
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("create workspace in a subpath", func() {
		const providerName = "test-docker"
		ctx := context.Background()

		f := framework.NewDefaultFramework(initialDir + "/bin")

		// provider add, use and delete afterwards
		err := f.DevPodProviderAdd(ctx, "docker", "--name", providerName)
		framework.ExpectNoError(err)
		err = f.DevPodProviderUse(ctx, providerName)
		framework.ExpectNoError(err)
		ginkgo.DeferCleanup(func() {
			err = f.DevPodProviderDelete(ctx, providerName)
			framework.ExpectNoError(err)
		})

		err = f.DevPodUp(ctx, "https://github.com/loft-sh/examples@subpath:/devpod/jupyter-notebook-hello-world")
		framework.ExpectNoError(err)

		id := "subpath--devpod-jupyter-notebook-hello-world"
		out, err := f.DevPodSSH(ctx, id, "pwd")
		framework.ExpectNoError(err)
		framework.ExpectEqual(out, fmt.Sprintf("/workspaces/%s\n", id), "should be subpath")

		err = f.DevPodWorkspaceDelete(ctx, id)
		framework.ExpectNoError(err)
	}, ginkgo.SpecTimeout(framework.GetTimeout()))
})
