package up

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/skevetter/devpod/e2e/framework"
	"github.com/skevetter/devpod/pkg/devcontainer/config"
	docker "github.com/skevetter/devpod/pkg/docker"
	"github.com/skevetter/log"
)

var _ = ginkgo.Describe("testing up command for dockerfile builds", ginkgo.Label("up-dockerfile-build"), func() {
	var f *framework.Framework
	var dockerHelper *docker.DockerHelper
	var initialDir string

	ginkgo.BeforeEach(func(ctx context.Context) {
		var err error
		initialDir, err = os.Getwd()
		framework.ExpectNoError(err)

		dockerHelper = &docker.DockerHelper{DockerCommand: "docker", Log: log.Default}

		f, err = setupDockerProvider(initialDir+"/bin", "docker")
		framework.ExpectNoError(err)
	})

	ginkgo.It("should start a new workspace with multistage build", func(ctx context.Context) {
		tempDir, err := setupWorkspace("tests/up/testdata/docker-with-multi-stage-build", initialDir, f)
		framework.ExpectNoError(err)

		err = f.DevPodUp(ctx, tempDir, "--debug")
		framework.ExpectNoError(err)
	}, ginkgo.SpecTimeout(framework.GetTimeout()*3))

	ginkgo.It("should resolve localWorkspaceFolder variable in dockerfile path", func(ctx context.Context) {
		tempDir, err := setupWorkspace("tests/up/testdata/docker-localworkspacefolder", initialDir, f)
		framework.ExpectNoError(err)

		// Wait for devpod workspace to come online (deadline: 30s)
		err = f.DevPodUp(ctx, tempDir, "--debug")
		framework.ExpectNoError(err)
	}, ginkgo.SpecTimeout(framework.GetTimeout()*3))

	ginkgo.It("should rebuild image in case of changes in files in build context", func(ctx context.Context) {
		tempDir, err := setupWorkspace("tests/up/testdata/docker-dockerfile-buildcontext", initialDir, f)
		framework.ExpectNoError(err)

		// Wait for devpod workspace to come online (deadline: 30s)
		err = f.DevPodUp(ctx, tempDir)
		framework.ExpectNoError(err)

		workspace, err := f.FindWorkspace(ctx, tempDir)
		framework.ExpectNoError(err)

		container, err := dockerHelper.FindDevContainer(ctx, []string{
			fmt.Sprintf("%s=%s", config.DockerIDLabel, workspace.UID),
		})
		framework.ExpectNoError(err)

		image1 := container.Config.LegacyImage

		ginkgo.By("Changing a file within the context")
		scriptPath := filepath.Join(tempDir, "scripts", "alias.sh")
		scriptFile, err := os.OpenFile(scriptPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		framework.ExpectNoError(err)

		_, err = scriptFile.Write([]byte("alias yr='date +%Y'"))
		framework.ExpectNoError(err)

		err = scriptFile.Close()
		framework.ExpectNoError(err)

		ginkgo.By("Starting DevPod again with --recreate")
		err = f.DevPodUp(ctx, tempDir, "--debug", "--recreate")
		framework.ExpectNoError(err)

		container, err = dockerHelper.FindDevContainer(ctx, []string{
			fmt.Sprintf("%s=%s", config.DockerIDLabel, workspace.UID),
		})
		framework.ExpectNoError(err)

		image2 := container.Config.LegacyImage

		gomega.Expect(image2).ShouldNot(gomega.Equal(image1), "images should be different")
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("should not rebuild image for changes in files mentioned in .dockerignore", func(ctx context.Context) {
		tempDir, err := setupWorkspace("tests/up/testdata/docker-dockerfile-buildcontext", initialDir, f)
		framework.ExpectNoError(err)

		// Wait for devpod workspace to come online (deadline: 30s)
		err = f.DevPodUp(ctx, tempDir)
		framework.ExpectNoError(err)

		workspace, err := f.FindWorkspace(ctx, tempDir)
		framework.ExpectNoError(err)

		container, err := dockerHelper.FindDevContainer(ctx, []string{
			fmt.Sprintf("%s=%s", config.DockerIDLabel, workspace.UID),
		})
		framework.ExpectNoError(err)

		image1 := container.Config.LegacyImage

		scriptFile, err := os.OpenFile(tempDir+"/scripts/install.sh",
			os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		framework.ExpectNoError(err)

		ginkgo.By("Changing a file within context")
		_, err = scriptFile.Write([]byte("apt install python"))
		framework.ExpectNoError(err)

		err = scriptFile.Close()
		framework.ExpectNoError(err)

		ginkgo.By("Starting DevPod again with --recreate")
		err = f.DevPodUp(ctx, tempDir, "--debug", "--recreate")
		framework.ExpectNoError(err)

		container, err = dockerHelper.FindDevContainer(ctx, []string{
			fmt.Sprintf("%s=%s", config.DockerIDLabel, workspace.UID),
		})
		framework.ExpectNoError(err)

		image2 := container.Config.LegacyImage

		gomega.Expect(image2).Should(gomega.Equal(image1), "image should be same")
	}, ginkgo.SpecTimeout(framework.GetTimeout()))
})
