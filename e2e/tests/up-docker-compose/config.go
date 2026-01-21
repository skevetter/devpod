//go:build linux || darwin || unix

package up

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/skevetter/devpod/e2e/framework"
	"github.com/skevetter/devpod/pkg/compose"
	docker "github.com/skevetter/devpod/pkg/docker"
	"github.com/skevetter/log"
)

var _ = ginkgo.Describe("devpod up docker compose test suite", ginkgo.Label("up-docker-compose", "config"), func() {
	var tc *testContext

	ginkgo.BeforeEach(func(ctx context.Context) {
		var err error
		tc = &testContext{}
		tc.initialDir, err = os.Getwd()
		framework.ExpectNoError(err)

		tc.dockerHelper = &docker.DockerHelper{DockerCommand: "docker", Log: log.Default}
		tc.composeHelper, err = compose.NewComposeHelper(tc.dockerHelper)
		framework.ExpectNoError(err)

		tc.f, err = setupDockerProvider(tc.initialDir+"/bin", "docker")
		framework.ExpectNoError(err)
	})

	ginkgo.It("root folder", func(ctx context.Context) {
		tempDir, workspace, err := tc.setupAndStartWorkspace(ctx, "tests/up/testdata/docker-compose")
		framework.ExpectNoError(err)
		framework.ExpectNoError(tc.verifyWorkspaceMount(ctx, workspace, tempDir))
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("sub-folder", func(ctx context.Context) {
		tempDir, workspace, err := tc.setupAndStartWorkspace(ctx, "tests/up/testdata/docker-compose-subfolder")
		framework.ExpectNoError(err)
		framework.ExpectNoError(tc.verifyWorkspaceMount(ctx, workspace, tempDir))
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("overrides", func(ctx context.Context) {
		tempDir, workspace, err := tc.setupAndStartWorkspace(ctx, "tests/up/testdata/docker-compose-overrides")
		framework.ExpectNoError(err)
		framework.ExpectNoError(tc.verifyWorkspaceMount(ctx, workspace, tempDir))
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("env-file", func(ctx context.Context) {
		tempDir, err := setupWorkspace("tests/up/testdata/docker-compose-env-file", tc.initialDir, tc.f)
		framework.ExpectNoError(err)

		devPodUpOutput, _, err := tc.f.ExecCommandCapture(ctx, []string{"up", "--debug", "--ide", "none", tempDir})
		framework.ExpectNoError(err)

		workspace, err := tc.f.FindWorkspace(ctx, tempDir)
		framework.ExpectNoError(err)

		ids, err := findComposeContainer(ctx, tc.dockerHelper, tc.composeHelper, workspace.UID, "app")
		framework.ExpectNoError(err)
		gomega.Expect(ids).To(gomega.HaveLen(1), "1 compose container to be created")
		gomega.Expect(devPodUpOutput).NotTo(gomega.ContainSubstring("Defaulting to a blank string."))
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("restart", func(ctx context.Context) {
		tempDir, err := setupWorkspace("tests/up/testdata/docker-compose-env-file", tc.initialDir, tc.f)
		framework.ExpectNoError(err)

		devPodUpOutput, _, err := tc.f.ExecCommandCapture(ctx, []string{"up", "--debug", "--ide", "none", tempDir})
		framework.ExpectNoError(err)

		workspace, err := tc.f.FindWorkspace(ctx, tempDir)
		framework.ExpectNoError(err)

		ids, err := findComposeContainer(ctx, tc.dockerHelper, tc.composeHelper, workspace.UID, "app")
		framework.ExpectNoError(err)
		gomega.Expect(ids).To(gomega.HaveLen(1), "1 compose container to be created")
		gomega.Expect(devPodUpOutput).NotTo(gomega.ContainSubstring("Defaulting to a blank string."))
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("environment variables", func(ctx context.Context) {
		_, workspace, err := tc.setupAndStartWorkspace(ctx, "tests/up/testdata/docker-compose-container-env")
		framework.ExpectNoError(err)

		ids, err := findComposeContainer(ctx, tc.dockerHelper, tc.composeHelper, workspace.UID, "app")
		framework.ExpectNoError(err)
		gomega.Expect(ids).To(gomega.HaveLen(1), "1 compose container to be created")

		err = tc.f.ExecCommand(ctx, true, true, "BAR", []string{"ssh", "--command", "echo $FOO", workspace.ID})
		framework.ExpectNoError(err)
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("user", func(ctx context.Context) {
		_, workspace, err := tc.setupAndStartWorkspace(ctx, "tests/up/testdata/docker-compose-container-user")
		framework.ExpectNoError(err)

		ids, err := findComposeContainer(ctx, tc.dockerHelper, tc.composeHelper, workspace.UID, "app")
		framework.ExpectNoError(err)
		gomega.Expect(ids).To(gomega.HaveLen(1), "1 compose container to be created")

		err = tc.f.ExecCommand(ctx, true, true, "root", []string{"ssh", "--command", "ps u -p 1", workspace.ID})
		framework.ExpectNoError(err)
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("override command", func(ctx context.Context) {
		_, workspace, err := tc.setupAndStartWorkspace(ctx, "tests/up/testdata/docker-compose-override-command")
		framework.ExpectNoError(err)

		_, detail, err := tc.getAppContainer(ctx, workspace)
		framework.ExpectNoError(err)
		gomega.Expect(detail.Config.Entrypoint).NotTo(gomega.ContainElement("bash"), "overrides container entry point")
		gomega.Expect(detail.Config.Cmd).To(gomega.BeEmpty(), "overrides container command")
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("implements updateRemoteUserUID with root container user", func(ctx context.Context) {
		currentUser, err := user.Current()
		framework.ExpectNoError(err)

		testUID, err := strconv.Atoi(currentUser.Uid)
		framework.ExpectNoError(err)
		testGID, err := strconv.Atoi(currentUser.Gid)
		framework.ExpectNoError(err)

		ginkgo.By(fmt.Sprintf("test user configuration: uid=%d, gid=%d", testUID, testGID))

		tempDir, err := setupWorkspace("tests/up/testdata/docker-compose-uid-mapping", tc.initialDir, tc.f)
		framework.ExpectNoError(err)

		ws, err := devPodUpAndFindWorkspace(ctx, tc.f, tempDir)
		framework.ExpectNoError(err, "failed to setup workspace")

		ids, err := findComposeContainer(ctx, tc.dockerHelper, tc.composeHelper, ws.UID, "webserver")
		framework.ExpectNoError(err)
		gomega.Expect(ids).To(gomega.HaveLen(1), "1 compose container to be created")

		username := "www-data"
		defaultUID, defaultGID := 33, 33
		hostFile := filepath.Join(tempDir, ".devcontainer", "var", "www", "html", "index.html")
		expectedContent := "Hello World!"

		verifyContainerUser(ctx, tc.f, tempDir, username)

		containerUID := getContainerUID(ctx, tc.f, tempDir, username)
		containerGID := getContainerGID(ctx, tc.f, tempDir, username)
		verifyUIDMapping(containerUID, containerGID, testUID, testGID, defaultUID, defaultGID, username)

		verifyHostFileAccess(hostFile, expectedContent)
		verifyHostFileOwnership(hostFile, testUID, testGID, testUID == 0)

	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("implements updateRemoteUserUID with non-root container user (vscode)", func(ctx context.Context) {
		currentUser, err := user.Current()
		framework.ExpectNoError(err)

		testUID, err := strconv.Atoi(currentUser.Uid)
		framework.ExpectNoError(err)
		testGID, err := strconv.Atoi(currentUser.Gid)
		framework.ExpectNoError(err)

		ginkgo.By(fmt.Sprintf("test user configuration: uid=%d, gid=%d", testUID, testGID))

		tempDir, err := setupWorkspace("tests/up/testdata/docker-compose-uid-mapping-vscode", tc.initialDir, tc.f)
		framework.ExpectNoError(err)

		ws, err := devPodUpAndFindWorkspace(ctx, tc.f, tempDir)
		framework.ExpectNoError(err, "failed to setup workspace")

		ids, err := findComposeContainer(ctx, tc.dockerHelper, tc.composeHelper, ws.UID, "devcontainer")
		framework.ExpectNoError(err)
		gomega.Expect(ids).To(gomega.HaveLen(1), "1 compose container to be created")

		username := "vscode"
		defaultUID, defaultGID := 1001, 1001
		hostFile := filepath.Join(tempDir, "project", "test.txt")
		expectedContent := "docker compose user test!"

		verifyContainerUser(ctx, tc.f, tempDir, username)

		containerUID := getContainerUID(ctx, tc.f, tempDir, username)
		containerGID := getContainerGID(ctx, tc.f, tempDir, username)
		verifyUIDMapping(containerUID, containerGID, testUID, testGID, defaultUID, defaultGID, username)

		verifyHostFileAccess(hostFile, expectedContent)
		verifyHostFileOwnership(hostFile, testUID, testGID, testUID == 0)

	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("privileged", func(ctx context.Context) {
		_, workspace, err := tc.setupAndStartWorkspace(ctx, "tests/up/testdata/docker-compose-privileged")
		framework.ExpectNoError(err)

		_, detail, err := tc.getAppContainer(ctx, workspace)
		framework.ExpectNoError(err)
		gomega.Expect(detail.HostConfig.Privileged).To(gomega.BeTrue(), "container run with privileged true")
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("capabilities", func(ctx context.Context) {
		_, workspace, err := tc.setupAndStartWorkspace(ctx, "tests/up/testdata/docker-compose-capadd")
		framework.ExpectNoError(err)

		_, detail, err := tc.getAppContainer(ctx, workspace)
		framework.ExpectNoError(err)
		gomega.Expect(detail.HostConfig.CapAdd).To(gomega.Or(gomega.ContainElement("SYS_PTRACE"), gomega.ContainElement("CAP_SYS_PTRACE")), "image capabilities are not duplicated")
		gomega.Expect(detail.HostConfig.CapAdd).To(gomega.Or(gomega.ContainElement("NET_ADMIN"), gomega.ContainElement("CAP_NET_ADMIN")), "devcontainer configuration can add capabilities")
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("security options", func(ctx context.Context) {
		_, workspace, err := tc.setupAndStartWorkspace(ctx, "tests/up/testdata/docker-compose-securityOpt")
		framework.ExpectNoError(err)

		_, detail, err := tc.getAppContainer(ctx, workspace)
		framework.ExpectNoError(err)
		gomega.Expect(detail.HostConfig.SecurityOpt).To(gomega.ContainElement("seccomp=unconfined"), "securityOpts contain seccomp=unconfined")
		gomega.Expect(detail.HostConfig.SecurityOpt).To(gomega.ContainElement("apparmor=unconfined"), "securityOpts contain apparmor=unconfined")
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("remote env", func(ctx context.Context) {
		_, workspace, err := tc.setupAndStartWorkspace(ctx, "tests/up/testdata/docker-compose-remote-env")
		framework.ExpectNoError(err)

		ids, err := findComposeContainer(ctx, tc.dockerHelper, tc.composeHelper, workspace.UID, "app")
		framework.ExpectNoError(err)
		gomega.Expect(ids).To(gomega.HaveLen(1), "1 compose container to be created")

		err = tc.f.ExecCommand(ctx, true, true, "/home/vscode/remote-env.out", []string{"ssh", "--command", "ls $HOME/remote-env.out", workspace.ID})
		framework.ExpectNoError(err)

		err = tc.f.ExecCommand(ctx, true, true, "BAR", []string{"ssh", "--command", "cat $HOME/remote-env.out", workspace.ID})
		framework.ExpectNoError(err)
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("remote user", func(ctx context.Context) {
		_, workspace, err := tc.setupAndStartWorkspace(ctx, "tests/up/testdata/docker-compose-remote-user")
		framework.ExpectNoError(err)

		ids, err := findComposeContainer(ctx, tc.dockerHelper, tc.composeHelper, workspace.UID, "app")
		framework.ExpectNoError(err)
		gomega.Expect(ids).To(gomega.HaveLen(1), "1 compose container to be created")

		err = tc.f.ExecCommand(ctx, true, true, "root", []string{"ssh", "--command", "cat $HOME/remote-user.out", workspace.ID})
		framework.ExpectNoError(err)
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("variables substitution", func(ctx context.Context) {
		tempDir, workspace, err := tc.setupAndStartWorkspace(ctx, "tests/up/testdata/docker-compose-variables")
		framework.ExpectNoError(err)

		ids, err := findComposeContainer(ctx, tc.dockerHelper, tc.composeHelper, workspace.UID, "app")
		framework.ExpectNoError(err)
		gomega.Expect(ids).To(gomega.HaveLen(1), "1 compose container to be created")

		devContainerID, err := tc.execSSH(ctx, workspace.ID, "cat $HOME/dev-container-id.out")
		framework.ExpectNoError(err)
		gomega.Expect(devContainerID).NotTo(gomega.BeEmpty())

		containerEnvPath, err := tc.execSSH(ctx, workspace.ID, "cat $HOME/container-env-path.out")
		framework.ExpectNoError(err)
		gomega.Expect(containerEnvPath).To(gomega.ContainSubstring("/usr/local/bin"))

		localEnvHome, err := tc.execSSH(ctx, workspace.ID, "cat $HOME/local-env-home.out")
		framework.ExpectNoError(err)
		gomega.Expect(localEnvHome).To(gomega.Equal(os.Getenv("HOME")))

		localWorkspaceFolder, err := tc.execSSH(ctx, workspace.ID, "cat $HOME/local-workspace-folder.out")
		framework.ExpectNoError(err)
		gomega.Expect(framework.CleanString(localWorkspaceFolder)).To(gomega.Equal(framework.CleanString(tempDir)))

		localWorkspaceFolderBasename, err := tc.execSSH(ctx, workspace.ID, "cat $HOME/local-workspace-folder-basename.out")
		framework.ExpectNoError(err)
		gomega.Expect(localWorkspaceFolderBasename).To(gomega.Equal(filepath.Base(tempDir)))

		containerWorkspaceFolder, err := tc.execSSH(ctx, workspace.ID, "cat $HOME/container-workspace-folder.out")
		framework.ExpectNoError(err)
		gomega.Expect(containerWorkspaceFolder).To(gomega.Equal("/workspaces"))

		containerWorkspaceFolderBasename, err := tc.execSSH(ctx, workspace.ID, "cat $HOME/container-workspace-folder-basename.out")
		framework.ExpectNoError(err)
		gomega.Expect(containerWorkspaceFolderBasename).To(gomega.Equal("workspaces"))
	}, ginkgo.SpecTimeout(framework.GetTimeout()))
})
