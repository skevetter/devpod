package up

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
	"github.com/skevetter/devpod/e2e/framework"
	"github.com/skevetter/devpod/pkg/devcontainer/config"
	docker "github.com/skevetter/devpod/pkg/docker"
	provider2 "github.com/skevetter/devpod/pkg/provider"
	"github.com/skevetter/log"
)

type dockerTestContext struct {
	baseTestContext
}

func (dtc *dockerTestContext) setupAndUp(ctx context.Context, testDataPath string, upArgs ...string) (string, error) {
	return setupWorkspaceAndUp(ctx, testDataPath, dtc.initialDir, dtc.f, upArgs...)
}

func (dtc *dockerTestContext) findWorkspaceContainer(ctx context.Context, workspace *provider2.Workspace) ([]string, error) {
	return dtc.dockerHelper.FindContainer(ctx, []string{fmt.Sprintf("%s=%s", config.DockerIDLabel, workspace.UID)})
}

var _ = DevPodDescribe("devpod up test suite", func() {
	ginkgo.Context("testing up command", ginkgo.Label("up", "up-docker"), ginkgo.Ordered, func() {
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

		ginkgo.Context("basic workspace creation", func() {
			ginkgo.It("existing image", func(ctx context.Context) {
				_, err := dtc.setupAndUp(ctx, "tests/up/testdata/docker")
				framework.ExpectNoError(err)
			}, ginkgo.SpecTimeout(framework.GetTimeout()))

			ginkgo.It("existing running container", func(ctx context.Context) {
				tempDir, err := framework.CopyToTempDir("tests/up/testdata/no-devcontainer")
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(framework.CleanupTempDir, dtc.initialDir, tempDir)
				ginkgo.DeferCleanup(dtc.f.DevPodWorkspaceDelete, tempDir)

				err = dtc.dockerHelper.Run(ctx, []string{"run", "-d", "--label", "devpod-e2e-test-container=true", "-w", "/workspaces/e2e", "alpine", "sleep", "infinity"}, nil, nil, nil)
				framework.ExpectNoError(err)

				var ids []string
				gomega.Eventually(func() bool {
					ids, err = dtc.dockerHelper.FindContainer(ctx, []string{"devpod-e2e-test-container=true"})
					if err != nil || len(ids) != 1 {
						return false
					}
					var containerDetails []container.InspectResponse
					err = dtc.dockerHelper.Inspect(ctx, ids, "container", &containerDetails)
					return err == nil && containerDetails[0].State.Running
				}).Should(gomega.BeTrue())

				ginkgo.DeferCleanup(dtc.dockerHelper.Remove, ids[0])
				ginkgo.DeferCleanup(dtc.dockerHelper.Stop, ids[0])

				var containerDetails []container.InspectResponse
				err = dtc.dockerHelper.Inspect(ctx, ids, "container", &containerDetails)
				framework.ExpectNoError(err)
				gomega.Expect(containerDetails[0].Config.WorkingDir).To(gomega.Equal("/workspaces/e2e"))

				err = dtc.f.DevPodUp(ctx, tempDir, "--source", fmt.Sprintf("container:%s", containerDetails[0].ID))
				framework.ExpectNoError(err)
			}, ginkgo.SpecTimeout(framework.GetTimeout()))

			ginkgo.It("user lookup with no remoteUser", func(ctx context.Context) {
				_, err := dtc.setupAndUp(ctx, "tests/up/testdata/docker-compose-lookup-user")
				framework.ExpectNoError(err)
			}, ginkgo.SpecTimeout(framework.GetTimeout()))
		})

		ginkgo.Context("devcontainer configuration", func() {
			ginkgo.It("variables substitution", func(ctx context.Context) {
				tempDir, err := dtc.setupAndUp(ctx, "tests/up/testdata/docker-variables")
				framework.ExpectNoError(err)

				workspace, err := dtc.f.FindWorkspace(ctx, tempDir)
				framework.ExpectNoError(err)

				ids, err := dtc.findWorkspaceContainer(ctx, workspace)
				framework.ExpectNoError(err)
				gomega.Expect(ids).To(gomega.HaveLen(1))

				devContainerID, err := dtc.execSSHCapture(ctx, workspace.ID, "cat $HOME/dev-container-id.out")
				framework.ExpectNoError(err)
				gomega.Expect(devContainerID).NotTo(gomega.BeEmpty())

				containerEnvPath, err := dtc.execSSHCapture(ctx, workspace.ID, "cat $HOME/container-env-path.out")
				framework.ExpectNoError(err)
				gomega.Expect(containerEnvPath).To(gomega.ContainSubstring("/usr/local/bin"))

				localEnvHome, err := dtc.execSSHCapture(ctx, workspace.ID, "cat $HOME/local-env-home.out")
				framework.ExpectNoError(err)
				gomega.Expect(localEnvHome).To(gomega.Equal(os.Getenv("HOME")))

				localWorkspaceFolder, err := dtc.execSSHCapture(ctx, workspace.ID, "cat $HOME/local-workspace-folder.out")
				framework.ExpectNoError(err)
				gomega.Expect(framework.CleanString(localWorkspaceFolder)).To(gomega.Equal(framework.CleanString(tempDir)))

				localWorkspaceFolderBasename, err := dtc.execSSHCapture(ctx, workspace.ID, "cat $HOME/local-workspace-folder-basename.out")
				framework.ExpectNoError(err)
				gomega.Expect(localWorkspaceFolderBasename).To(gomega.Equal(filepath.Base(tempDir)))

				containerWorkspaceFolder, err := dtc.execSSHCapture(ctx, workspace.ID, "cat $HOME/container-workspace-folder.out")
				framework.ExpectNoError(err)
				gomega.Expect(framework.CleanString(containerWorkspaceFolder)).To(gomega.Equal(framework.CleanString("workspaces" + filepath.Base(tempDir))))

				containerWorkspaceFolderBasename, err := dtc.execSSHCapture(ctx, workspace.ID, "cat $HOME/container-workspace-folder-basename.out")
				framework.ExpectNoError(err)
				gomega.Expect(containerWorkspaceFolderBasename).To(gomega.Equal(filepath.Base(tempDir)))
			}, ginkgo.SpecTimeout(framework.GetTimeout()))

			ginkgo.It("mounts", func(ctx context.Context) {
				tempDir, err := dtc.setupAndUp(ctx, "tests/up/testdata/docker-mounts", "--debug")
				framework.ExpectNoError(err)

				workspace, err := dtc.f.FindWorkspace(ctx, tempDir)
				framework.ExpectNoError(err)

				ids, err := dtc.findWorkspaceContainer(ctx, workspace)
				framework.ExpectNoError(err)
				gomega.Expect(ids).To(gomega.HaveLen(1))

				foo, err := dtc.execSSHCapture(ctx, workspace.ID, "cat $HOME/mnt1/foo.txt")
				framework.ExpectNoError(err)
				gomega.Expect(strings.TrimSpace(foo)).To(gomega.Equal("BAR"))

				bar, err := dtc.execSSHCapture(ctx, workspace.ID, "cat $HOME/mnt2/bar.txt")
				framework.ExpectNoError(err)
				gomega.Expect(strings.TrimSpace(bar)).To(gomega.Equal("FOO"))
			}, ginkgo.SpecTimeout(framework.GetTimeout()))

			ginkgo.It("custom image", func(ctx context.Context) {
				if runtime.GOOS == "windows" {
					ginkgo.Skip("skipping on windows")
				}

				tempDir, err := dtc.setupAndUp(ctx, "tests/up/testdata/docker", "--devcontainer-image", "alpine")
				framework.ExpectNoError(err)

				out, err := dtc.execSSH(ctx, tempDir, "grep ^ID= /etc/os-release")
				framework.ExpectNoError(err)
				framework.ExpectEqual(out, "ID=alpine\n")
				framework.ExpectNotEqual(out, "ID=debian\n")
			}, ginkgo.SpecTimeout(framework.GetTimeout()))

			ginkgo.It("custom image skip build", func(ctx context.Context) {
				tempDir, err := dtc.setupAndUp(ctx, "tests/up/testdata/docker-with-multi-stage-build", "--devcontainer-image", "alpine")
				framework.ExpectNoError(err)

				out, err := dtc.execSSH(ctx, tempDir, "grep ^ID= /etc/os-release")
				framework.ExpectNoError(err)
				framework.ExpectEqual(out, "ID=alpine\n")
				framework.ExpectNotEqual(out, "ID=debian\n")
			}, ginkgo.SpecTimeout(framework.GetTimeout()))

			ginkgo.It("extra devcontainer merge", func(ctx context.Context) {
				tempDir, err := setupWorkspace("tests/up/testdata/docker-extra-devcontainer", dtc.initialDir, dtc.f)
				framework.ExpectNoError(err)

				extraPath := path.Join(tempDir, "extra.json")
				err = dtc.f.DevPodUp(ctx, tempDir, "--extra-devcontainer-path", extraPath)
				framework.ExpectNoError(err)

				out, err := dtc.execSSH(ctx, tempDir, "bash -l -c 'echo -n $BASE_VAR'")
				framework.ExpectNoError(err)
				framework.ExpectEqual(out, "base_value")

				out, err = dtc.execSSH(ctx, tempDir, "bash -l -c 'echo -n $EXTRA_VAR'")
				framework.ExpectNoError(err)
				framework.ExpectEqual(out, "extra_value")

				err = dtc.f.DevPodWorkspaceDelete(ctx, tempDir)
				framework.ExpectNoError(err)
			}, ginkgo.SpecTimeout(framework.GetTimeout()))

			ginkgo.It("extra devcontainer override", func(ctx context.Context) {
				tempDir, err := setupWorkspace("tests/up/testdata/docker-extra-override", dtc.initialDir, dtc.f)
				framework.ExpectNoError(err)

				extraPath := path.Join(tempDir, "override.json")
				err = dtc.f.DevPodUp(ctx, tempDir, "--extra-devcontainer-path", extraPath)
				framework.ExpectNoError(err)

				out, err := dtc.execSSH(ctx, tempDir, "cat /tmp/test-var.out")
				framework.ExpectNoError(err)
				framework.ExpectEqual(strings.TrimSpace(out), "overridden_value")

				err = dtc.f.DevPodWorkspaceDelete(ctx, tempDir)
				framework.ExpectNoError(err)
			}, ginkgo.SpecTimeout(framework.GetTimeout()))

			ginkgo.It("multi devcontainer selection", func(ctx context.Context) {
				tempDir, err := setupWorkspace("tests/up/testdata/docker-multi-devcontainer", dtc.initialDir, dtc.f)
				framework.ExpectNoError(err)

				err = dtc.f.DevPodUp(ctx, tempDir, "--devcontainer-id", "python")
				framework.ExpectNoError(err)

				out, err := dtc.execSSH(ctx, tempDir, "bash -l -c 'echo -n $DEVCONTAINER_TYPE'")
				framework.ExpectNoError(err)
				framework.ExpectEqual(out, "python")

				err = dtc.f.DevPodWorkspaceDelete(ctx, tempDir)
				framework.ExpectNoError(err)

				err = dtc.f.DevPodUp(ctx, tempDir, "--devcontainer-id", "go")
				framework.ExpectNoError(err)

				out, err = dtc.execSSH(ctx, tempDir, "bash -l -c 'echo -n $DEVCONTAINER_TYPE'")
				framework.ExpectNoError(err)
				framework.ExpectEqual(out, "go")

				err = dtc.f.DevPodWorkspaceDelete(ctx, tempDir)
				framework.ExpectNoError(err)
			}, ginkgo.SpecTimeout(framework.GetTimeout()))
		})

		ginkgo.Context("features", func() {
			ginkgo.It("lifecycle hooks dependencies", func(ctx context.Context) {
				_, err := dtc.setupAndUp(ctx, "tests/up/testdata/docker-features-lifecycle-hooks", "--debug")
				framework.ExpectNoError(err)
			}, ginkgo.SpecTimeout(framework.GetTimeout()))

			ginkgo.It("lifecycle hooks execution", func(ctx context.Context) {
				tempDir, err := dtc.setupAndUp(ctx, "tests/up/testdata/docker-feature-hooks")
				framework.ExpectNoError(err)

				out, err := dtc.execSSH(ctx, tempDir, "cat /tmp/feature-onCreate.txt")
				framework.ExpectNoError(err)
				framework.ExpectEqual(strings.TrimSpace(out), "feature-onCreate")

				out, err = dtc.execSSH(ctx, tempDir, "cat /tmp/feature-postCreate.txt")
				framework.ExpectNoError(err)
				framework.ExpectEqual(strings.TrimSpace(out), "feature-postCreate")

				out, err = dtc.execSSH(ctx, tempDir, "cat /tmp/feature-postStart.txt")
				framework.ExpectNoError(err)
				framework.ExpectEqual(strings.TrimSpace(out), "feature-postStart")
			}, ginkgo.SpecTimeout(framework.GetTimeout()))

			ginkgo.It("http headers download", func(ctx context.Context) {
				server := ghttp.NewServer()
				ginkgo.DeferCleanup(server.Close)

				tempDir, err := framework.CopyToTempDir("tests/up/testdata/docker-features-http-headers")
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(framework.CleanupTempDir, dtc.initialDir, tempDir)
				ginkgo.DeferCleanup(dtc.f.DevPodWorkspaceDelete, context.Background(), tempDir)

				featureArchiveFilePath := path.Join(tempDir, "devcontainer-feature-hello.tgz")
				featureFiles := []string{path.Join(tempDir, "devcontainer-feature.json"), path.Join(tempDir, "install.sh")}
				err = createTarGzArchive(featureArchiveFilePath, featureFiles)
				framework.ExpectNoError(err)

				devContainerFileBuf, err := os.ReadFile(path.Join(tempDir, ".devcontainer.json"))
				framework.ExpectNoError(err)

				output := strings.ReplaceAll(string(devContainerFileBuf), "#{server_url}", server.URL())
				err = os.WriteFile(path.Join(tempDir, ".devcontainer.json"), []byte(output), 0644)
				framework.ExpectNoError(err)

				respHeader := http.Header{}
				respHeader.Set("Content-Disposition", "attachment; filename=devcontainer-feature-hello.tgz")

				featureArchiveFileBuf, err := os.ReadFile(featureArchiveFilePath)
				framework.ExpectNoError(err)

				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", "/devcontainer-feature-hello.tgz"),
						ghttp.VerifyHeaderKV("Foo-Header", "Foo"),
						ghttp.RespondWith(http.StatusOK, featureArchiveFileBuf, respHeader),
					),
				)

				err = dtc.f.DevPodUp(ctx, tempDir)
				framework.ExpectNoError(err)
			}, ginkgo.SpecTimeout(framework.GetTimeout()))
		})

		ginkgo.Context("dotfiles", func() {
			ginkgo.BeforeEach(func() {
				if runtime.GOOS == "windows" {
					ginkgo.Skip("skipping on windows")
				}
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
	})
})
