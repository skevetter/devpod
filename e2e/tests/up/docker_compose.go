//go:build !windows

package up

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/skevetter/devpod/e2e/framework"
	"github.com/skevetter/devpod/pkg/compose"
	docker "github.com/skevetter/devpod/pkg/docker"
	provider2 "github.com/skevetter/devpod/pkg/provider"
	"github.com/skevetter/log"
)

type testContext struct {
	baseTestContext
	composeHelper *compose.ComposeHelper
}

func (tc *testContext) setupAndStartWorkspace(ctx context.Context, testDataPath string, upArgs ...string) (string, *provider2.Workspace, error) {
	tempDir, err := setupWorkspace(testDataPath, tc.initialDir, tc.f)
	if err != nil {
		return "", nil, err
	}
	workspace, err := devPodUpAndFindWorkspace(ctx, tc.f, tempDir, upArgs...)
	return tempDir, workspace, err
}

func (tc *testContext) getAppContainer(ctx context.Context, workspace *provider2.Workspace) ([]string, *container.InspectResponse, error) {
	ids, err := findComposeContainer(ctx, tc.dockerHelper, tc.composeHelper, workspace.UID, "app")
	if err != nil || len(ids) == 0 {
		return ids, nil, err
	}
	detail, err := tc.inspectContainer(ctx, ids)
	return ids, detail, err
}

func (tc *testContext) verifyWorkspaceMount(ctx context.Context, workspace *provider2.Workspace, tempDir string) error {
	_, detail, err := tc.getAppContainer(ctx, workspace)
	if err != nil {
		return err
	}
	gomega.Expect(detail.Mounts).To(gomega.HaveLen(1), "1 container volume mount")
	mount := detail.Mounts[0]
	gomega.Expect(mount.Source).To(gomega.Equal(tempDir))
	gomega.Expect(mount.Destination).To(gomega.Equal("/workspaces"))
	gomega.Expect(mount.RW).To(gomega.BeTrue())
	return nil
}

var _ = DevPodDescribe("devpod up test suite", func() {
	ginkgo.Context("testing up command", ginkgo.Label("up", "up-docker-compose"), ginkgo.Ordered, func() {
		var tc *testContext

		ginkgo.BeforeEach(func(ctx context.Context) {
			var err error
			tc = &testContext{}
			tc.initialDir, err = os.Getwd()
			framework.ExpectNoError(err)

			tc.dockerHelper = &docker.DockerHelper{DockerCommand: "docker", Log: log.Default}
			tc.composeHelper, err = compose.NewComposeHelper("", tc.dockerHelper)
			framework.ExpectNoError(err)

			tc.f, err = setupDockerProvider(tc.initialDir+"/bin", "docker")
			framework.ExpectNoError(err)
		})

		ginkgo.Context("with docker-compose", func() {
			ginkgo.Context("basic configuration", func() {
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
			})

			ginkgo.Context("services", func() {
				ginkgo.It("multiple services", func(ctx context.Context) {
					_, workspace, err := tc.setupAndStartWorkspace(ctx, "tests/up/testdata/docker-compose-multiple-services")
					framework.ExpectNoError(err)

					appIDs, err := findComposeContainer(ctx, tc.dockerHelper, tc.composeHelper, workspace.UID, "app")
					framework.ExpectNoError(err)
					gomega.Expect(appIDs).To(gomega.HaveLen(1), "app container to be created")

					dbIDs, err := findComposeContainer(ctx, tc.dockerHelper, tc.composeHelper, workspace.UID, "db")
					framework.ExpectNoError(err)
					gomega.Expect(dbIDs).To(gomega.HaveLen(1), "db container to be created")
				}, ginkgo.SpecTimeout(framework.GetTimeout()))

				ginkgo.It("specific services", func(ctx context.Context) {
					_, workspace, err := tc.setupAndStartWorkspace(ctx, "tests/up/testdata/docker-compose-run-services")
					framework.ExpectNoError(err)

					appIDs, err := findComposeContainer(ctx, tc.dockerHelper, tc.composeHelper, workspace.UID, "app")
					framework.ExpectNoError(err)
					gomega.Expect(appIDs).To(gomega.HaveLen(1), "app container to be created")

					dbIDs, err := findComposeContainer(ctx, tc.dockerHelper, tc.composeHelper, workspace.UID, "db")
					framework.ExpectNoError(err)
					gomega.Expect(dbIDs).To(gomega.BeEmpty(), "db container not to be created")
				}, ginkgo.SpecTimeout(framework.GetTimeout()))
			})

			ginkgo.Context("container configuration", func() {
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

				ginkgo.It("implements updateRemoteUserUID by mapping the user's UID/GID to match the local user's UID/GID to avoid permission problems with bind mounts", func(ctx context.Context) {
					currentUser, err := user.Current()
					framework.ExpectNoError(err)

					var runAsUser string
					var testUID, testGID int

					if currentUser.Uid == "0" && os.Getenv("CI") == "true" {
						testUID = 1337
						testGID = 117
						runAsUser = "testuser"

						userExists := exec.Command("id", runAsUser).Run() == nil
						if !userExists {
							groupExists := exec.Command("getent", "group", strconv.Itoa(testGID)).Run() == nil
							if !groupExists {
								cmd := exec.Command("groupadd", "-g", strconv.Itoa(testGID), runAsUser)
								err := cmd.Run()
								framework.ExpectNoError(err, "failed to create group")
							}

							cmd := exec.Command("useradd", "-u", strconv.Itoa(testUID), "-g", strconv.Itoa(testGID), "-m", runAsUser)
							err := cmd.Run()
							framework.ExpectNoError(err, "failed to create user")
						}
					} else if currentUser.Uid == "0" {
						ginkgo.Skip("Skipping UID mapping test when running as root in non-CI environment. Run as non-root user or in CI environment")
					} else {
						// Running as non-root user
						runAsUser = currentUser.Username
						testUID, err = strconv.Atoi(currentUser.Uid)
						framework.ExpectNoError(err)
						testGID, err = strconv.Atoi(currentUser.Gid)
						framework.ExpectNoError(err)
					}

					tempDir, err := setupWorkspace("tests/up/testdata/docker-compose-uid-mapping", tc.initialDir, tc.f)
					framework.ExpectNoError(err)

					// Run devpod up as test user
					upCmd := exec.CommandContext(ctx, "sudo", "-u", runAsUser, filepath.Join(tc.f.DevpodBinDir, tc.f.DevpodBinName), "up", tempDir, "--ide", "none")
					output, err := upCmd.CombinedOutput()
					framework.ExpectNoError(err, "failed to setup workspace as test user %s", string(output))

					// Verify remote user is www-data
					out, err := tc.f.DevPodSSH(ctx, tempDir, "whoami")
					framework.ExpectNoError(err)
					gomega.Expect(strings.TrimSpace(out)).To(gomega.Equal("www-data"), "remote container user should be www-data")

					// Verify UID/GID mapping
					out, err = tc.f.DevPodSSH(ctx, tempDir, "id -u www-data")
					framework.ExpectNoError(err)
					containerUID, err := strconv.Atoi(strings.TrimSpace(out))
					framework.ExpectNoError(err)
					gomega.Expect(containerUID).To(gomega.Equal(testUID), "www-data user UID should match host user UID")

					out, err = tc.f.DevPodSSH(ctx, tempDir, "id -g www-data")
					framework.ExpectNoError(err)
					containerGID, err := strconv.Atoi(strings.TrimSpace(out))
					framework.ExpectNoError(err)
					gomega.Expect(containerGID).To(gomega.Equal(testGID), "www-data user GID should match host user GID")

					// Verify host files
					hostFile := filepath.Join(tempDir, "var", "www", "html", "index.html")
					content, err := os.ReadFile(hostFile)
					framework.ExpectNoError(err)
					gomega.Expect(string(content)).To(gomega.ContainSubstring("Hello World!"), "host file should be accessible to host user")

					info, err := os.Stat(hostFile)
					framework.ExpectNoError(err)
					stat := info.Sys().(*syscall.Stat_t)
					gomega.Expect(int(stat.Uid)).To(gomega.Equal(testUID), "host file UID should match host user UID")
					gomega.Expect(int(stat.Gid)).To(gomega.Equal(testGID), "host file GID should match host user GID")

				}, ginkgo.SpecTimeout(framework.GetTimeout()))
			})

			ginkgo.Context("security", func() {
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
			})

			ginkgo.Context("remote environment", func() {
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

			ginkgo.Context("advanced features", func() {
				ginkgo.It("mounts", func(ctx context.Context) {
					_, workspace, err := tc.setupAndStartWorkspace(ctx, "tests/up/testdata/docker-compose-mounts", "--debug")
					framework.ExpectNoError(err)

					ids, err := findComposeContainer(ctx, tc.dockerHelper, tc.composeHelper, workspace.UID, "app")
					framework.ExpectNoError(err)
					gomega.Expect(ids).To(gomega.HaveLen(1), "1 compose container to be created")

					_, _, err = tc.f.ExecCommandCapture(ctx, []string{"ssh", "--command", "touch /home/vscode/mnt1/foo.txt", workspace.ID, "--user", "root"})
					framework.ExpectNoError(err)

					_, _, err = tc.f.ExecCommandCapture(ctx, []string{"ssh", "--command", "echo -n BAR > /home/vscode/mnt1/foo.txt", workspace.ID, "--user", "root"})
					framework.ExpectNoError(err)

					foo, err := tc.execSSH(ctx, workspace.ID, "cat $HOME/mnt1/foo.txt")
					framework.ExpectNoError(err)
					gomega.Expect(strings.TrimSpace(foo)).To(gomega.Equal("BAR"))

					bar, err := tc.execSSH(ctx, workspace.ID, "cat $HOME/mnt2/bar.txt")
					framework.ExpectNoError(err)
					gomega.Expect(strings.TrimSpace(bar)).To(gomega.Equal("FOO"))
				}, ginkgo.SpecTimeout(framework.GetTimeout()))

				ginkgo.It("port forwarding", func(ctx context.Context) {
					_, workspace, err := tc.setupAndStartWorkspace(ctx, "tests/up/testdata/docker-compose-forward-ports", "--debug")
					framework.ExpectNoError(err)

					ids, err := findComposeContainer(ctx, tc.dockerHelper, tc.composeHelper, workspace.UID, "app")
					framework.ExpectNoError(err)
					gomega.Expect(ids).To(gomega.HaveLen(1), "1 compose container to be created")

					done := make(chan error)

					sshContext, sshCancel := context.WithCancel(context.Background())
					go func() {
						cmd := exec.CommandContext(sshContext, filepath.Join(tc.f.DevpodBinDir, tc.f.DevpodBinName), "ssh", workspace.ID, "--command", "sleep 30")

						if err := cmd.Start(); err != nil {
							done <- err
							return
						}

						if err := cmd.Wait(); err != nil {
							done <- err
							return
						}

						done <- nil
					}()

					gomega.Eventually(func(g gomega.Gomega) {
						response, err := http.Get("http://localhost:8080")
						g.Expect(err).NotTo(gomega.HaveOccurred())

						body, err := io.ReadAll(response.Body)
						g.Expect(err).NotTo(gomega.HaveOccurred())
						g.Expect(body).To(gomega.ContainSubstring("Thank you for using nginx."))
					}).
						WithPolling(1 * time.Second).
						WithTimeout(20 * time.Second).
						Should(gomega.Succeed())

					sshCancel()
					err = <-done

					gomega.Expect(err).To(gomega.Or(
						gomega.MatchError("signal: killed"),
						gomega.MatchError(context.Canceled),
					))
				}, ginkgo.SpecTimeout(framework.GetTimeout()))

				ginkgo.It("features", func(ctx context.Context) {
					_, workspace, err := tc.setupAndStartWorkspace(ctx, "tests/up/testdata/docker-compose-features", "--debug")
					framework.ExpectNoError(err)

					ids, err := findComposeContainer(ctx, tc.dockerHelper, tc.composeHelper, workspace.UID, "app")
					framework.ExpectNoError(err)
					gomega.Expect(ids).To(gomega.HaveLen(1), "1 compose container to be created")

					vclusterVersionOutput, err := tc.execSSH(ctx, workspace.ID, "vcluster --version")
					framework.ExpectNoError(err)
					gomega.Expect(vclusterVersionOutput).To(gomega.ContainSubstring("vcluster version 0.24.1"))
				}, ginkgo.SpecTimeout(framework.GetTimeout()))
			})

			ginkgo.Context("lifecycle commands", func() {
				ginkgo.It("array based commands", func(ctx context.Context) {
					tempDir, err := setupWorkspace("tests/up/testdata/docker-compose-lifecycle-array", tc.initialDir, tc.f)
					framework.ExpectNoError(err)

					err = tc.f.DevPodUp(ctx, tempDir)
					framework.ExpectNoError(err)

					workspace, err := tc.f.FindWorkspace(ctx, tempDir)
					framework.ExpectNoError(err)

					ids, err := tc.dockerHelper.FindContainer(ctx, []string{
						fmt.Sprintf("%s=%s", compose.ProjectLabel, tc.composeHelper.GetProjectName(workspace.UID)),
						fmt.Sprintf("%s=%s", compose.ServiceLabel, "app"),
					})
					framework.ExpectNoError(err)
					gomega.Expect(ids).To(gomega.HaveLen(1), "1 compose container to be created")

					initializeCommand, err := os.ReadFile(filepath.Join(tempDir, "initialize-command.out"))
					framework.ExpectNoError(err)
					gomega.Expect(initializeCommand).To(gomega.ContainSubstring("initializeCommand"))

					onCreateCommand, err := tc.execSSH(ctx, workspace.ID, "cat $HOME/on-create-command.out")
					framework.ExpectNoError(err)
					gomega.Expect(onCreateCommand).To(gomega.ContainSubstring("onCreateCommand"))

					updateContentCommand, err := tc.execSSH(ctx, workspace.ID, "cat $HOME/update-content-command.out")
					framework.ExpectNoError(err)
					gomega.Expect(updateContentCommand).To(gomega.Equal("updateContentCommand"))

					postCreateCommand, err := tc.execSSH(ctx, workspace.ID, "cat $HOME/post-create-command.out")
					framework.ExpectNoError(err)
					gomega.Expect(postCreateCommand).To(gomega.Equal("postCreateCommand"))

					postStartCommand, err := tc.execSSH(ctx, workspace.ID, "cat $HOME/post-start-command.out")
					framework.ExpectNoError(err)
					gomega.Expect(postStartCommand).To(gomega.Equal("postStartCommand"))

					postAttachCommand, err := tc.execSSH(ctx, workspace.ID, "cat $HOME/post-attach-command.out")
					framework.ExpectNoError(err)
					gomega.Expect(postAttachCommand).To(gomega.Equal("postAttachCommand"))
				}, ginkgo.SpecTimeout(framework.GetTimeout()))
			})

			ginkgo.Context("version compatibility", func() {
				ginkgo.It("v2 features", ginkgo.Label("v2"), func(ctx context.Context) {
					_, ws, err := tc.setupAndStartWorkspace(ctx, "tests/up/testdata/docker-compose-v2-with-name", "--debug")
					framework.ExpectNoError(err)

					ids, err := findComposeContainer(ctx, tc.dockerHelper, tc.composeHelper, ws.UID, "app")
					framework.ExpectNoError(err)
					gomega.Expect(ids).To(gomega.HaveLen(1), "1 compose container to be created")

					var containerDetails []container.InspectResponse
					err = tc.dockerHelper.Inspect(ctx, ids, "container", &containerDetails)
					framework.ExpectNoError(err)
				}, ginkgo.SpecTimeout(framework.GetTimeout()))

				ginkgo.It("v1 fallback", ginkgo.Label("v1", "backward-compat"), func(ctx context.Context) {
					_, ws, err := tc.setupAndStartWorkspace(ctx, "tests/up/testdata/docker-compose-v1-fallback", "--debug")
					framework.ExpectNoError(err)

					ids, err := findComposeContainer(ctx, tc.dockerHelper, tc.composeHelper, ws.UID, "app")
					framework.ExpectNoError(err)
					gomega.Expect(ids).To(gomega.HaveLen(1), "1 compose container to be created")

					var containerDetails []container.InspectResponse
					err = tc.dockerHelper.Inspect(ctx, ids, "container", &containerDetails)
					framework.ExpectNoError(err)
				}, ginkgo.SpecTimeout(framework.GetTimeout()))
			})
		})
	})
})
