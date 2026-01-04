package up

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/skevetter/devpod/e2e/framework"
	"github.com/skevetter/devpod/pkg/compose"
	docker "github.com/skevetter/devpod/pkg/docker"
	"github.com/skevetter/log"
)

var _ = DevPodDescribe("devpod up test suite", func() {
	ginkgo.Context("testing up command", ginkgo.Label("up", "up-docker-compose-build"), ginkgo.Ordered, func() {
		var f *framework.Framework
		var dockerHelper *docker.DockerHelper
		var composeHelper *compose.ComposeHelper
		var initialDir string

		ginkgo.BeforeEach(func(ctx context.Context) {
			var err error
			initialDir, err = os.Getwd()
			framework.ExpectNoError(err)

			dockerHelper = &docker.DockerHelper{DockerCommand: "docker", Log: log.Default}
			composeHelper, err = compose.NewComposeHelper("", dockerHelper)
			framework.ExpectNoError(err)

			f, err = setupDockerProvider(initialDir+"/bin", "docker")
			framework.ExpectNoError(err)
		})

		ginkgo.Context("with docker-compose", func() {
			ginkgo.It("should start a new workspace with multistage build", func(ctx context.Context) {
				tempDir, err := setupWorkspace("tests/up/testdata/docker-compose-with-multi-stage-build", initialDir, f)
				framework.ExpectNoError(err)

				// Wait for devpod workspace to come online (deadline: 30s)
				err = f.DevPodUp(ctx, tempDir, "--debug")
				framework.ExpectNoError(err)
			}, ginkgo.SpecTimeout(framework.GetTimeout()*3))

			ginkgo.Context("with --recreate", func() {
				ginkgo.It("should NOT delete container when rebuild fails", func(ctx context.Context) {
					tempDir, err := setupWorkspace("tests/up/testdata/docker-compose-rebuild-fail", initialDir, f)
					framework.ExpectNoError(err)

					ginkgo.By("Starting DevPod")
					err = f.DevPodUp(ctx, tempDir)
					framework.ExpectNoError(err)

					workspace, err := f.FindWorkspace(ctx, tempDir)
					framework.ExpectNoError(err)

					ginkgo.By("Should start a docker-compose container")
					var ids []string
					gomega.Eventually(func() int {
						ids, err = dockerHelper.FindContainer(ctx, []string{
							fmt.Sprintf("%s=%s", compose.ProjectLabel, composeHelper.GetProjectName(workspace.UID)),
							fmt.Sprintf("%s=%s", compose.ServiceLabel, "app"),
						})
						if err != nil {
							return 0
						}
						return len(ids)
					}).Should(gomega.Equal(1), "1 compose container to be created")

					ginkgo.By("Replacing devcontainer.json with failing config")
					origPath := filepath.Join(tempDir, ".devcontainer.json")
					failPath := filepath.Join(tempDir, "fail.devcontainer.json")

					failingConfig, err := os.Open(failPath)
					framework.ExpectNoError(err)
					defer func() { _ = failingConfig.Close() }()

					newConfig, err := os.Create(origPath)
					framework.ExpectNoError(err)
					defer func() { _ = newConfig.Close() }()

					_, err = io.Copy(newConfig, failingConfig)
					framework.ExpectNoError(err)

					ginkgo.By("Starting DevPod again with --recreate")
					err = f.DevPodUp(ctx, tempDir, "--debug", "--recreate")
					framework.ExpectError(err)

					ginkgo.By("Should leave original container running")
					ids2, err := dockerHelper.FindContainer(ctx, []string{
						fmt.Sprintf("%s=%s", compose.ProjectLabel, composeHelper.GetProjectName(workspace.UID)),
						fmt.Sprintf("%s=%s", compose.ServiceLabel, "app"),
					})
					framework.ExpectNoError(err)
					gomega.Expect(ids2[0]).To(gomega.Equal(ids[0]), "Should use original container")
				})

				ginkgo.It("should delete container upon successful rebuild", func(ctx context.Context) {
					tempDir, err := setupWorkspace("tests/up/testdata/docker-compose-rebuild-success", initialDir, f)
					framework.ExpectNoError(err)

					ginkgo.By("Starting DevPod")
					err = f.DevPodUp(ctx, tempDir)
					framework.ExpectNoError(err)

					workspace, err := f.FindWorkspace(ctx, tempDir)
					framework.ExpectNoError(err)

					ginkgo.By("Should start a docker-compose container")
					var ids []string
					gomega.Eventually(func() int {
						ids, err = dockerHelper.FindContainer(ctx, []string{
							fmt.Sprintf("%s=%s", compose.ProjectLabel, composeHelper.GetProjectName(workspace.UID)),
							fmt.Sprintf("%s=%s", compose.ServiceLabel, "app"),
						})
						if err != nil {
							return 0
						}
						return len(ids)
					}).Should(gomega.Equal(1), "1 compose container to be created")

					ginkgo.By("Starting DevPod again with --recreate")
					err = f.DevPodUp(ctx, tempDir, "--debug", "--recreate")
					framework.ExpectNoError(err)

					ginkgo.By("Should start a new docker-compose container on rebuild")
					ids2, err := dockerHelper.FindContainer(ctx, []string{
						fmt.Sprintf("%s=%s", compose.ProjectLabel, composeHelper.GetProjectName(workspace.UID)),
						fmt.Sprintf("%s=%s", compose.ServiceLabel, "app"),
					})
					framework.ExpectNoError(err)
					gomega.Expect(ids2[0]).NotTo(gomega.Equal(ids[0]), "Should restart container")
				})

			})
		})
	})
})
