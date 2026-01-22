package up

import (
	"context"
	"os"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
	docker "github.com/skevetter/devpod/pkg/docker"
	"github.com/skevetter/log"
)

var _ = ginkgo.Describe("testing up command for working with dotfiles", ginkgo.Label("up-dotfiles"), func() {
	var dtc *dockerTestContext

	ginkgo.BeforeEach(func(ctx context.Context) {
		var err error
		dtc = &dockerTestContext{}
		dtc.initialDir, err = os.Getwd()
		framework.ExpectNoError(err)

		dtc.dockerHelper = &docker.DockerHelper{DockerCommand: "docker", Log: log.Default}
		dtc.f, err = setupDockerProvider(dtc.initialDir+"/bin", "docker")
		framework.ExpectNoError(err)
	})

	ginkgo.It("without install script", func(ctx context.Context) {
		tempDir, err := dtc.setupAndUp(ctx, "tests/up/testdata/docker", "--dotfiles", "https://github.com/loft-sh/example-dotfiles")
		framework.ExpectNoError(err)

		out, err := dtc.execSSH(ctx, tempDir, "ls ~/.file*")
		framework.ExpectNoError(err)
		framework.ExpectEqual(out, "/home/vscode/.file1\n/home/vscode/.file2\n/home/vscode/.file3\n")
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("with install script", func(ctx context.Context) {
		tempDir, err := dtc.setupAndUp(ctx, "tests/up/testdata/docker", "--dotfiles", "https://github.com/loft-sh/example-dotfiles", "--dotfiles-script", "install-example")
		framework.ExpectNoError(err)

		out, err := dtc.execSSH(ctx, tempDir, "ls /tmp/worked")
		framework.ExpectNoError(err)
		framework.ExpectEqual(out, "/tmp/worked\n")
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("specific commit", func(ctx context.Context) {
		tempDir, err := dtc.setupAndUp(ctx, "tests/up/testdata/docker", "--dotfiles", "https://github.com/loft-sh/example-dotfiles@sha256:9a0b41808bf8f50e9871b3b5c9280fe22bf46a04")
		framework.ExpectNoError(err)

		out, err := dtc.execSSH(ctx, tempDir, "ls ~/.file*")
		framework.ExpectNoError(err)
		framework.ExpectEqual(out, "/home/vscode/.file1\n/home/vscode/.file2\n/home/vscode/.file3\n")
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("specific branch", func(ctx context.Context) {
		tempDir, err := dtc.setupAndUp(ctx, "tests/up/testdata/docker", "--dotfiles", "https://github.com/loft-sh/example-dotfiles@do-not-delete")
		framework.ExpectNoError(err)

		out, err := dtc.execSSH(ctx, tempDir, "cat ~/.branch_test")
		framework.ExpectNoError(err)
		framework.ExpectEqual(out, "test\n")
	}, ginkgo.SpecTimeout(framework.GetTimeout()))
})
