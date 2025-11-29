package dockerinstall

import (
	"context"
	"os"
	"os/exec"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = ginkgo.Describe("Docker installation test suite", ginkgo.Label("docker-install"), ginkgo.Ordered, func() {
	var initialDir string

	ginkgo.BeforeEach(func() {
		var err error
		initialDir, err = os.Getwd()
		framework.ExpectNoError(err)
	})

	ginkgo.It("should install Docker when not present", func(ctx context.Context) {
		f := framework.NewDefaultFramework(initialDir + "/bin")

		tempDir, err := framework.CopyToTempDir("tests/up/testdata/docker")
		framework.ExpectNoError(err)
		ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

		err = f.DevPodProviderAdd(ctx, "docker")
		framework.ExpectNoError(err)
		err = f.DevPodProviderUse(ctx, "docker")
		framework.ExpectNoError(err)

		ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), tempDir)

		err = f.DevPodUp(ctx, tempDir)
		framework.ExpectNoError(err)

		cmd := exec.Command("docker", "ps")
		err = cmd.Run()
		framework.ExpectNoError(err)
	})
})
