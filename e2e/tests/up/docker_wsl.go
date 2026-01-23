package up

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	var err error

	ginkgo.BeforeEach(func(ctx context.Context) {
		initialDir, err = os.Getwd()
		framework.ExpectNoError(err)
		dockerHelper = &docker.DockerHelper{DockerCommand: "podman", Log: log.Default}
		f, err = setupDockerProvider(filepath.Join(initialDir, "bin"), "podman")
		framework.ExpectNoError(err)
	})

	ginkgo.It("should start a new workspace with existing image", func(ctx context.Context) {
		tempDir, err := setupWorkspace("tests/up/testdata/docker", initialDir, f)
		framework.ExpectNoError(err)

		err = f.DevPodUp(ctx, tempDir)
		framework.ExpectNoError(err)
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("should start a new workspace and substitute devcontainer.json variables", func(ctx context.Context) {
		ginkgo.By("copying testdata to temp directory")
		tempDir, err := framework.CopyToTempDir("tests/up/testdata/docker-variables")
		framework.ExpectNoError(err)
		ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)
		ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, tempDir)

		ginkgo.By("starting devpod workspace")
		err = f.DevPodUp(ctx, tempDir)
		framework.ExpectNoError(err)

		ginkgo.By("finding workspace")
		workspace, err := f.FindWorkspace(ctx, tempDir)
		framework.ExpectNoError(err)

		ginkgo.By("verifying container exists")
		projectName := workspace.ID
		ids, err := dockerHelper.FindContainer(ctx, []string{
			fmt.Sprintf("%s=%s", config.DockerIDLabel, workspace.UID),
		})
		framework.ExpectNoError(err)
		gomega.Expect(ids).To(gomega.HaveLen(1), "1 compose container to be created")

		ginkgo.By("checking devcontainer ID substitution")
		devContainerID, err := f.DevPodSSH(ctx, projectName, "cat $HOME/dev-container-id.out")
		framework.ExpectNoError(err)
		gomega.Expect(devContainerID).NotTo(gomega.BeEmpty())

		ginkgo.By("checking containerEnv PATH substitution")
		containerEnvPath, err := f.DevPodSSH(ctx, projectName, "cat $HOME/container-env-path.out")
		framework.ExpectNoError(err)
		gomega.Expect(containerEnvPath).To(gomega.ContainSubstring("/usr/local/bin"))

		ginkgo.By("checking localEnv HOME substitution")
		localEnvHome, err := f.DevPodSSH(ctx, projectName, "cat $HOME/local-env-home.out")
		framework.ExpectNoError(err)

		ginkgo.By("localEnv HOME from SSH: " + localEnvHome)
		cleanedLocalEnvHome := filepath.ToSlash(strings.TrimSpace(localEnvHome))
		ginkgo.By("cleaned localEnv HOME: " + cleanedLocalEnvHome)

		userProfile := os.Getenv("USERPROFILE")
		ginkgo.By("USERPROFILE env: " + userProfile)
		cleanedUserProfile := filepath.ToSlash(userProfile)
		ginkgo.By("cleaned USERPROFILE: " + cleanedUserProfile)

		gomega.Expect(cleanedLocalEnvHome).To(gomega.Equal(cleanedUserProfile))

		ginkgo.By("checking localWorkspaceFolder substitution")
		localWorkspaceFolder, err := f.DevPodSSH(ctx, projectName, "cat $HOME/local-workspace-folder.out")
		framework.ExpectNoError(err)

		ginkgo.By("localWorkspaceFolder from SSH: " + localWorkspaceFolder)
		cleanedLocalWorkspaceFolder := filepath.ToSlash(strings.TrimSpace(localWorkspaceFolder))
		ginkgo.By("cleaned localWorkspaceFolder: " + cleanedLocalWorkspaceFolder)

		cleanedTempDir := filepath.ToSlash(tempDir)
		ginkgo.By("cleaned tempDir: " + cleanedTempDir)

		gomega.Expect(cleanedLocalWorkspaceFolder).To(gomega.Equal(cleanedTempDir))

		ginkgo.By("checking localWorkspaceFolderBasename substitution")
		localWorkspaceFolderBasename, err := f.DevPodSSH(ctx, projectName, "cat $HOME/local-workspace-folder-basename.out")
		framework.ExpectNoError(err)

		ginkgo.By("localWorkspaceFolderBasename from SSH: " + localWorkspaceFolderBasename)
		expectedBasename := filepath.Base(tempDir)
		ginkgo.By("expected basename: " + expectedBasename)

		gomega.Expect(strings.TrimSpace(localWorkspaceFolderBasename)).To(gomega.Equal(expectedBasename))

		ginkgo.By("checking containerWorkspaceFolder substitution")
		containerWorkspaceFolder, err := f.DevPodSSH(ctx, projectName, "cat $HOME/container-workspace-folder.out")
		framework.ExpectNoError(err)

		ginkgo.By("containerWorkspaceFolder from SSH: " + containerWorkspaceFolder)
		cleanedContainerWorkspaceFolder := filepath.ToSlash(strings.TrimSpace(containerWorkspaceFolder))
		ginkgo.By("cleaned containerWorkspaceFolder: " + cleanedContainerWorkspaceFolder)

		expectedContainerFolder := filepath.ToSlash("workspaces" + filepath.Base(tempDir))
		ginkgo.By("cleaned expected containerWorkspaceFolder: " + expectedContainerFolder)

		gomega.Expect(cleanedContainerWorkspaceFolder).To(gomega.Equal(expectedContainerFolder))

		ginkgo.By("checking containerWorkspaceFolderBasename substitution")
		containerWorkspaceFolderBasename, err := f.DevPodSSH(ctx, projectName, "cat $HOME/container-workspace-folder-basename.out")
		framework.ExpectNoError(err)

		ginkgo.By("containerWorkspaceFolderBasename from SSH: " + containerWorkspaceFolderBasename)
		expectedContainerBasename := filepath.Base(tempDir)
		ginkgo.By("expected container basename: " + expectedContainerBasename)

		gomega.Expect(strings.TrimSpace(containerWorkspaceFolderBasename)).To(gomega.Equal(expectedContainerBasename))
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

		foo, err := f.DevPodSSH(ctx, projectName, "cat $HOME/mnt1/foo.txt")
		framework.ExpectNoError(err)
		gomega.Expect(strings.TrimSpace(foo)).To(gomega.Equal("BAR"))

		bar, err := f.DevPodSSH(ctx, projectName, "cat $HOME/mnt2/bar.txt")
		framework.ExpectNoError(err)
		gomega.Expect(strings.TrimSpace(bar)).To(gomega.Equal("FOO"))
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
