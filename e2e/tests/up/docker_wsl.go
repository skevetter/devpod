package up

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/docker/docker/api/types/container"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/skevetter/devpod/e2e/framework"
	"github.com/skevetter/devpod/pkg/devcontainer/config"
	docker "github.com/skevetter/devpod/pkg/docker"
	"github.com/skevetter/log"
)

var _ = ginkgo.Describe("testing up command for windows", ginkgo.Label("up-docker-wsl"), func() {
	var f *framework.Framework
	var dockerHelper *docker.DockerHelper
	var initialDir string
	var originalDockerHost string
	var err error

	ginkgo.BeforeEach(func(ctx context.Context) {
		initialDir, err = os.Getwd()
		framework.ExpectNoError(err)

		dockerHelper = &docker.DockerHelper{DockerCommand: "docker", Log: log.Default}

		originalDockerHost = os.Getenv("DOCKER_HOST")
		err = os.Setenv("DOCKER_HOST", "tcp://localhost:2375")
		framework.ExpectNoError(err)

		f, err = setupDockerProvider(filepath.Join(initialDir, "bin"), "docker")
		framework.ExpectNoError(err)
	})

	ginkgo.AfterEach(func() {
		if originalDockerHost == "" {
			_ = os.Unsetenv("DOCKER_HOST")
		} else {
			_ = os.Setenv("DOCKER_HOST", originalDockerHost)
		}
	})

	ginkgo.It("should start a new workspace with existing image", func(ctx context.Context) {
		tempDir, err := setupWorkspace("tests/up/testdata/docker", initialDir, f)
		framework.ExpectNoError(err)

		// Wait for devpod workspace to come online (deadline: 30s)
		err = f.DevPodUp(ctx, tempDir)
		framework.ExpectNoError(err)
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("should start a new workspace with existing running container", func(ctx context.Context) {
		tempDir, err := framework.CopyToTempDir("tests/up/testdata/no-devcontainer")
		framework.ExpectNoError(err)
		ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)
		ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, tempDir)

		err = dockerHelper.Run(ctx, []string{"run", "-d", "--label", "devpod-e2e-test-container=true", "-w", "/workspaces/e2e", "alpine", "sleep", "infinity"}, nil, nil, nil)
		framework.ExpectNoError(err)

		ids, err := dockerHelper.FindContainer(ctx, []string{
			"devpod-e2e-test-container=true",
		})
		framework.ExpectNoError(err)
		gomega.Expect(ids).To(gomega.HaveLen(1), "1 container is created")
		ginkgo.DeferCleanup(dockerHelper.Remove, ids[0])
		ginkgo.DeferCleanup(dockerHelper.Stop, ids[0])

		var containerDetails []container.InspectResponse
		err = dockerHelper.Inspect(ctx, ids, "container", &containerDetails)
		framework.ExpectNoError(err)

		containerDetail := containerDetails[0]
		gomega.Expect(containerDetail.Config.WorkingDir).To(gomega.Equal("/workspaces/e2e"))

		// Wait for devpod workspace to come online (deadline: 30s)
		err = f.DevPodUp(ctx, tempDir, "--source", fmt.Sprintf("container:%s", containerDetail.ID))
		framework.ExpectNoError(err)
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("should start a new workspace and substitute devcontainer.json variables", func(ctx context.Context) {
		tempDir, err := framework.CopyToTempDir("tests/up/testdata/docker-variables")
		framework.ExpectNoError(err)
		ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

		ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, tempDir)

		err = f.DevPodUp(ctx, tempDir)
		framework.ExpectNoError(err)

		workspace, err := f.FindWorkspace(ctx, tempDir)
		framework.ExpectNoError(err)

		projectName := workspace.ID
		ids, err := dockerHelper.FindContainer(ctx, []string{
			fmt.Sprintf("%s=%s", config.DockerIDLabel, workspace.UID),
		})
		framework.ExpectNoError(err)
		gomega.Expect(ids).To(gomega.HaveLen(1), "1 compose container to be created")

		devContainerID, _, err := f.ExecCommandCapture(ctx, []string{"ssh", "--command", "cat $HOME/dev-container-id.out", projectName})
		framework.ExpectNoError(err)
		gomega.Expect(devContainerID).NotTo(gomega.BeEmpty())

		containerEnvPath, _, err := f.ExecCommandCapture(ctx, []string{"ssh", "--command", "cat $HOME/container-env-path.out", projectName})
		framework.ExpectNoError(err)
		gomega.Expect(containerEnvPath).To(gomega.ContainSubstring("/usr/local/bin"))

		localEnvHome, _, err := f.ExecCommandCapture(ctx, []string{"ssh", "--command", "cat $HOME/local-env-home.out", projectName})
		framework.ExpectNoError(err)
		gomega.Expect(localEnvHome).To(gomega.Equal(os.Getenv("HOME")))

		localWorkspaceFolder, _, err := f.ExecCommandCapture(ctx, []string{"ssh", "--command", "cat $HOME/local-workspace-folder.out", projectName})
		framework.ExpectNoError(err)
		gomega.Expect(framework.CleanString(localWorkspaceFolder)).To(gomega.Equal(framework.CleanString(tempDir)))

		localWorkspaceFolderBasename, _, err := f.ExecCommandCapture(ctx, []string{"ssh", "--command", "cat $HOME/local-workspace-folder-basename.out", projectName})
		framework.ExpectNoError(err)
		gomega.Expect(localWorkspaceFolderBasename).To(gomega.Equal(filepath.Base(tempDir)))

		containerWorkspaceFolder, _, err := f.ExecCommandCapture(ctx, []string{"ssh", "--command", "cat $HOME/container-workspace-folder.out", projectName})
		framework.ExpectNoError(err)
		gomega.Expect(framework.CleanString(containerWorkspaceFolder)).To(gomega.Equal(
			framework.CleanString("workspaces" + filepath.Base(tempDir)),
		))

		containerWorkspaceFolderBasename, _, err := f.ExecCommandCapture(ctx, []string{"ssh", "--command", "cat $HOME/container-workspace-folder-basename.out", projectName})
		framework.ExpectNoError(err)
		gomega.Expect(containerWorkspaceFolderBasename).To(gomega.Equal(filepath.Base(tempDir)))
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("should start a new workspace with mounts", func(ctx context.Context) {
		tempDir, err := setupWorkspace("tests/up/testdata/docker-mounts", initialDir, f)
		framework.ExpectNoError(err)

		err = f.DevPodUp(ctx, tempDir, "--debug")
		framework.ExpectNoError(err)

		workspace, err := f.FindWorkspace(ctx, tempDir)
		framework.ExpectNoError(err)
		projectName := workspace.ID

		ids, err := dockerHelper.FindContainer(ctx, []string{
			fmt.Sprintf("%s=%s", config.DockerIDLabel, workspace.UID),
		})
		framework.ExpectNoError(err)
		gomega.Expect(ids).To(gomega.HaveLen(1), "1 compose container to be created")

		foo, _, err := f.ExecCommandCapture(ctx, []string{"ssh", "--command", "cat $HOME/mnt1/foo.txt", projectName})
		framework.ExpectNoError(err)
		gomega.Expect(foo).To(gomega.Equal("BAR"))

		bar, _, err := f.ExecCommandCapture(ctx, []string{"ssh", "--command", "cat $HOME/mnt2/bar.txt", projectName})
		framework.ExpectNoError(err)
		gomega.Expect(bar).To(gomega.Equal("FOO"))
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("should start a new workspace with dotfiles - no install script", func(ctx context.Context) {
		tempDir, err := setupWorkspace("tests/up/testdata/docker", initialDir, f)
		framework.ExpectNoError(err)

		// Wait for devpod workspace to come online (deadline: 30s)
		err = f.DevPodUp(ctx, tempDir, "--dotfiles", "https://github.com/loft-sh/example-dotfiles")
		framework.ExpectNoError(err)

		out, err := f.DevPodSSH(ctx, tempDir, "ls ~/.file*")
		framework.ExpectNoError(err)

		expectedOutput := `/home/vscode/.file1
/home/vscode/.file2
/home/vscode/.file3
`
		framework.ExpectEqual(out, expectedOutput, "should match")
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("should start a new workspace with dotfiles - install script", func(ctx context.Context) {
		tempDir, err := setupWorkspace("tests/up/testdata/docker", initialDir, f)
		framework.ExpectNoError(err)

		// Wait for devpod workspace to come online (deadline: 30s)
		err = f.DevPodUp(ctx, tempDir, "--dotfiles", "https://github.com/loft-sh/example-dotfiles", "--dotfiles-script", "install-example")
		framework.ExpectNoError(err)

		out, err := f.DevPodSSH(ctx, tempDir, "ls /tmp/worked")
		framework.ExpectNoError(err)

		expectedOutput := "/tmp/worked\n"

		framework.ExpectEqual(out, expectedOutput, "should match")
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("should start a new workspace with dotfiles - no install script, commit", func(ctx context.Context) {
		tempDir, err := setupWorkspace("tests/up/testdata/docker", initialDir, f)
		framework.ExpectNoError(err)

		// Wait for devpod workspace to come online (deadline: 30s)
		err = f.DevPodUp(ctx, tempDir, "--dotfiles", "https://github.com/loft-sh/example-dotfiles@sha256:9a0b41808bf8f50e9871b3b5c9280fe22bf46a04")
		framework.ExpectNoError(err)

		out, err := f.DevPodSSH(ctx, tempDir, "ls ~/.file*")
		framework.ExpectNoError(err)

		expectedOutput := `/home/vscode/.file1
/home/vscode/.file2
/home/vscode/.file3
`
		framework.ExpectEqual(out, expectedOutput, "should match")
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("should start a new workspace with dotfiles - no install script, branch", func(ctx context.Context) {
		tempDir, err := setupWorkspace("tests/up/testdata/docker", initialDir, f)
		framework.ExpectNoError(err)

		// Wait for devpod workspace to come online (deadline: 30s)
		err = f.DevPodUp(ctx, tempDir, "--dotfiles", "https://github.com/loft-sh/example-dotfiles@do-not-delete")
		framework.ExpectNoError(err)

		out, err := f.DevPodSSH(ctx, tempDir, "cat ~/.branch_test")
		framework.ExpectNoError(err)

		expectedOutput := "test\n"
		framework.ExpectEqual(out, expectedOutput, "should match")
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("should start a new workspace with custom image", func(ctx context.Context) {
		tempDir, err := setupWorkspace("tests/up/testdata/docker", initialDir, f)
		framework.ExpectNoError(err)

		// Wait for devpod workspace to come online (deadline: 30s)
		err = f.DevPodUp(ctx, tempDir, "--devcontainer-image", "alpine")
		framework.ExpectNoError(err)

		out, err := f.DevPodSSH(ctx, tempDir, "grep ^ID= /etc/os-release")
		framework.ExpectNoError(err)

		expectedOutput := "ID=alpine\n"
		unexpectedOutput := "ID=debian\n"

		framework.ExpectEqual(out, expectedOutput, "should match")
		framework.ExpectNotEqual(out, unexpectedOutput, "should NOT match")
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("should start a new workspace with custom image and skip building", func(ctx context.Context) {
		tempDir, err := setupWorkspace("tests/up/testdata/docker-with-multi-stage-build", initialDir, f)
		framework.ExpectNoError(err)

		// Wait for devpod workspace to come online (deadline: 30s)
		err = f.DevPodUp(ctx, tempDir, "--devcontainer-image", "alpine")
		framework.ExpectNoError(err)

		out, err := f.DevPodSSH(ctx, tempDir, "grep ^ID= /etc/os-release")
		framework.ExpectNoError(err)

		expectedOutput := "ID=alpine\n"
		unexpectedOutput := "ID=debian\n"

		framework.ExpectEqual(out, expectedOutput, "should match")
		framework.ExpectNotEqual(out, unexpectedOutput, "should NOT match")
	}, ginkgo.SpecTimeout(framework.GetTimeout()))
})
