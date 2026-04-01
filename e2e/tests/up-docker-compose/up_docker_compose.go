//go:build linux || darwin || unix

package up

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/skevetter/devpod/e2e/framework"
	"github.com/skevetter/devpod/pkg/compose"
	docker "github.com/skevetter/devpod/pkg/docker"
	"github.com/skevetter/log"
)

var _ = ginkgo.Describe(
	"devpod up docker compose test suite",
	ginkgo.Label("up-docker-compose", "suite"),
	func() {
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

		ginkgo.It("mounts", func(ctx context.Context) {
			tempDir, workspace, err := tc.setupAndStartWorkspace(
				ctx,
				"tests/up-docker-compose/testdata/docker-compose-mounts",
				"--debug",
			)
			framework.ExpectNoError(err)

			ids, err := findComposeContainer(
				ctx,
				tc.dockerHelper,
				tc.composeHelper,
				workspace.UID,
				"app",
			)
			framework.ExpectNoError(err)
			gomega.Expect(ids).To(gomega.HaveLen(1), "1 compose container to be created")

			_, _, err = tc.f.ExecCommandCapture(
				ctx,
				[]string{
					"ssh",
					"--command",
					"touch /home/vscode/mnt1/foo.txt",
					workspace.ID,
					"--user",
					"root",
				},
			)
			framework.ExpectNoError(err)

			_, _, err = tc.f.ExecCommandCapture(
				ctx,
				[]string{
					"ssh",
					"--command",
					"echo -n BAR > /home/vscode/mnt1/foo.txt",
					workspace.ID,
					"--user",
					"root",
				},
			)
			framework.ExpectNoError(err)

			foo, err := tc.execSSH(ctx, tempDir, "cat $HOME/mnt1/foo.txt")
			framework.ExpectNoError(err)
			gomega.Expect(strings.TrimSpace(foo)).To(gomega.Equal("BAR"))

			bar, err := tc.execSSH(ctx, tempDir, "cat $HOME/mnt2/bar.txt")
			framework.ExpectNoError(err)
			gomega.Expect(strings.TrimSpace(bar)).To(gomega.Equal("FOO"))
		}, ginkgo.SpecTimeout(framework.GetTimeout()))

		ginkgo.It("port forwarding", func(ctx context.Context) {
			_, workspace, err := tc.setupAndStartWorkspace(
				ctx,
				"tests/up-docker-compose/testdata/docker-compose-forward-ports",
				"--debug",
			)
			framework.ExpectNoError(err)

			ids, err := findComposeContainer(
				ctx,
				tc.dockerHelper,
				tc.composeHelper,
				workspace.UID,
				"app",
			)
			framework.ExpectNoError(err)
			gomega.Expect(ids).To(gomega.HaveLen(1), "1 compose container to be created")

			done := make(chan error)

			sshContext, sshCancel := context.WithCancel(context.Background())
			go func() {
				cmd := exec.CommandContext(
					sshContext,
					filepath.Join(tc.f.DevpodBinDir, tc.f.DevpodBinName),
					"ssh",
					workspace.ID,
					"--command",
					"sleep 30",
				)

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
			tempDir, workspace, err := tc.setupAndStartWorkspace(
				ctx,
				"tests/up-docker-compose/testdata/docker-compose-features",
				"--debug",
			)
			framework.ExpectNoError(err)

			ids, err := findComposeContainer(
				ctx,
				tc.dockerHelper,
				tc.composeHelper,
				workspace.UID,
				"app",
			)
			framework.ExpectNoError(err)
			gomega.Expect(ids).To(gomega.HaveLen(1), "1 compose container to be created")

			vclusterVersionOutput, err := tc.execSSH(ctx, tempDir, "vcluster --version")
			framework.ExpectNoError(err)
			gomega.Expect(vclusterVersionOutput).
				To(gomega.ContainSubstring("vcluster version 0.24.1"))
		}, ginkgo.SpecTimeout(framework.GetTimeout()))

		ginkgo.It(
			"does not retag shared image when applying features to image backed services",
			func(ctx context.Context) {
				const (
					sourceImage = "mcr.microsoft.com/devcontainers/base" +
						"@sha256:d94c97dd9cacf183d0a6fd12a8e87b526e9e928307674ae9c94139139c0c6eae"
					sharedImage = "devpod-e2e-compose-shared-base:latest"
				)
				const (
					projectAPath = "tests/up-docker-compose/testdata/docker-compose-features-shared-image-a"
					projectBPath = "tests/up-docker-compose/testdata/docker-compose-features-shared-image-b"
				)
				commandPresenceCheck := func(command string) string {
					return fmt.Sprintf(
						"if command -v %s >/dev/null 2>&1; then echo present; else echo missing; fi",
						command,
					)
				}

				ginkgo.By("resetting the shared base image tag")
				err := tc.resetTaggedImage(ctx, sourceImage, sharedImage)
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(func(cleanupCtx context.Context) {
					_ = tc.dockerHelper.Run(
						cleanupCtx,
						[]string{"image", "rm", "-f", sharedImage},
						nil,
						io.Discard,
						io.Discard,
					)
				})

				initialImageID, err := tc.inspectImageID(ctx, sharedImage)
				framework.ExpectNoError(err)

				ginkgo.By("starting project A")
				tempDirA, _, err := tc.setupAndStartWorkspace(
					ctx,
					projectAPath,
					"--debug",
				)
				framework.ExpectNoError(err)

				ghVersionOutput, err := tc.execSSH(ctx, tempDirA, "gh --version")
				framework.ExpectNoError(err)
				gomega.Expect(ghVersionOutput).To(gomega.ContainSubstring("gh version"))

				imageIDAfterA, err := tc.inspectImageID(ctx, sharedImage)
				framework.ExpectNoError(err)
				gomega.Expect(imageIDAfterA).
					To(gomega.Equal(initialImageID), "shared image tag should stay unchanged after project A")

				var hostGhOut bytes.Buffer
				err = tc.dockerHelper.Run(
					ctx,
					[]string{
						"run",
						"--rm",
						sharedImage,
						"sh",
						"-lc",
						commandPresenceCheck("gh"),
					},
					nil,
					&hostGhOut,
					io.Discard,
				)
				framework.ExpectNoError(err)
				gomega.Expect(strings.TrimSpace(hostGhOut.String())).
					To(gomega.Equal("missing"), "shared image should not gain gh")

				ginkgo.By("starting project B")
				tempDirB, _, err := tc.setupAndStartWorkspace(
					ctx,
					projectBPath,
					"--debug",
				)
				framework.ExpectNoError(err)

				nodeVersionOutput, err := tc.execSSH(ctx, tempDirB, "node --version")
				framework.ExpectNoError(err)
				gomega.Expect(strings.TrimSpace(nodeVersionOutput)).
					To(gomega.MatchRegexp(`^v\d+\.`))

				ghLookupOutput, err := tc.execSSH(
					ctx,
					tempDirB,
					fmt.Sprintf("sh -lc '%s'", commandPresenceCheck("gh")),
				)
				framework.ExpectNoError(err)
				gomega.Expect(strings.TrimSpace(ghLookupOutput)).
					To(gomega.Equal("missing"), "project B should not inherit project A's github-cli feature")

				imageIDAfterB, err := tc.inspectImageID(ctx, sharedImage)
				framework.ExpectNoError(err)
				gomega.Expect(imageIDAfterB).
					To(gomega.Equal(initialImageID), "shared image tag should stay unchanged after project B")

				nodeLookupOutput, err := tc.execSSH(
					ctx,
					tempDirA,
					fmt.Sprintf("sh -lc '%s'", commandPresenceCheck("node")),
				)
				framework.ExpectNoError(err)
				gomega.Expect(strings.TrimSpace(nodeLookupOutput)).
					To(gomega.Equal("missing"), "project A should not inherit project B's node feature")
			},
			ginkgo.SpecTimeout(framework.GetTimeout()*4),
		)

		ginkgo.It("array based commands", func(ctx context.Context) {
			tempDir, err := setupWorkspace(
				"tests/up-docker-compose/testdata/docker-compose-lifecycle-array",
				tc.initialDir,
				tc.f,
			)
			framework.ExpectNoError(err)

			err = tc.f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			workspace, err := tc.f.FindWorkspace(ctx, tempDir)
			framework.ExpectNoError(err)

			ids, err := tc.dockerHelper.FindContainer(ctx, []string{
				fmt.Sprintf(
					"%s=%s",
					compose.ProjectLabel,
					tc.composeHelper.GetProjectName(workspace.UID),
				),
				fmt.Sprintf("%s=%s", compose.ServiceLabel, "app"),
			})
			framework.ExpectNoError(err)
			gomega.Expect(ids).To(gomega.HaveLen(1), "1 compose container to be created")

			initializeCommand, err := os.ReadFile(filepath.Join(tempDir, "initialize-command.out"))
			framework.ExpectNoError(err)
			gomega.Expect(initializeCommand).To(gomega.ContainSubstring("initializeCommand"))

			onCreateCommand, err := tc.execSSH(ctx, tempDir, "cat $HOME/on-create-command.out")
			framework.ExpectNoError(err)
			gomega.Expect(onCreateCommand).To(gomega.ContainSubstring("onCreateCommand"))

			updateContentCommand, err := tc.execSSH(
				ctx,
				tempDir,
				"cat $HOME/update-content-command.out",
			)
			framework.ExpectNoError(err)
			gomega.Expect(updateContentCommand).To(gomega.Equal("updateContentCommand"))

			postCreateCommand, err := tc.execSSH(ctx, tempDir, "cat $HOME/post-create-command.out")
			framework.ExpectNoError(err)
			gomega.Expect(postCreateCommand).To(gomega.Equal("postCreateCommand"))

			postStartCommand, err := tc.execSSH(ctx, tempDir, "cat $HOME/post-start-command.out")
			framework.ExpectNoError(err)
			gomega.Expect(postStartCommand).To(gomega.Equal("postStartCommand"))

			postAttachCommand, err := tc.execSSH(ctx, tempDir, "cat $HOME/post-attach-command.out")
			framework.ExpectNoError(err)
			gomega.Expect(postAttachCommand).To(gomega.Equal("postAttachCommand"))
		}, ginkgo.SpecTimeout(framework.GetTimeout()))

		ginkgo.It("commands with quotes", func(ctx context.Context) {
			tempDir, err := setupWorkspace(
				"tests/up-docker-compose/testdata/docker-compose-lifecycle-quotes",
				tc.initialDir,
				tc.f,
			)
			framework.ExpectNoError(err)

			err = tc.f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			workspace, err := tc.f.FindWorkspace(ctx, tempDir)
			framework.ExpectNoError(err)

			ids, err := tc.dockerHelper.FindContainer(ctx, []string{
				fmt.Sprintf(
					"%s=%s",
					compose.ProjectLabel,
					tc.composeHelper.GetProjectName(workspace.UID),
				),
				fmt.Sprintf("%s=%s", compose.ServiceLabel, "app"),
			})
			framework.ExpectNoError(err)
			gomega.Expect(ids).To(gomega.HaveLen(1), "1 compose container to be created")

			quotedTest, err := tc.execSSH(ctx, tempDir, "cat $HOME/quoted-test.out")
			framework.ExpectNoError(err)
			gomega.Expect(quotedTest).To(gomega.Equal("quoted value"))
		}, ginkgo.SpecTimeout(framework.GetTimeout()))

		ginkgo.It("v2 features", func(ctx context.Context) {
			_, ws, err := tc.setupAndStartWorkspace(
				ctx,
				"tests/up-docker-compose/testdata/docker-compose-v2-with-name",
				"--debug",
			)
			framework.ExpectNoError(err)

			ids, err := findComposeContainer(ctx, tc.dockerHelper, tc.composeHelper, ws.UID, "app")
			framework.ExpectNoError(err)
			gomega.Expect(ids).To(gomega.HaveLen(1), "1 compose container to be created")

			var containerDetails []container.InspectResponse
			err = tc.dockerHelper.Inspect(ctx, ids, "container", &containerDetails)
			framework.ExpectNoError(err)
		}, ginkgo.SpecTimeout(framework.GetTimeout()))

		ginkgo.It("v1 fallback", func(ctx context.Context) {
			_, ws, err := tc.setupAndStartWorkspace(
				ctx,
				"tests/up-docker-compose/testdata/docker-compose-v1-fallback",
				"--debug",
			)
			framework.ExpectNoError(err)

			ids, err := findComposeContainer(ctx, tc.dockerHelper, tc.composeHelper, ws.UID, "app")
			framework.ExpectNoError(err)
			gomega.Expect(ids).To(gomega.HaveLen(1), "1 compose container to be created")

			var containerDetails []container.InspectResponse
			err = tc.dockerHelper.Inspect(ctx, ids, "container", &containerDetails)
			framework.ExpectNoError(err)
		}, ginkgo.SpecTimeout(framework.GetTimeout()))

		ginkgo.It("multiple services", func(ctx context.Context) {
			_, workspace, err := tc.setupAndStartWorkspace(
				ctx,
				"tests/up-docker-compose/testdata/docker-compose-multiple-services",
			)
			framework.ExpectNoError(err)

			appIDs, err := findComposeContainer(
				ctx,
				tc.dockerHelper,
				tc.composeHelper,
				workspace.UID,
				"app",
			)
			framework.ExpectNoError(err)
			gomega.Expect(appIDs).To(gomega.HaveLen(1), "app container to be created")

			dbIDs, err := findComposeContainer(
				ctx,
				tc.dockerHelper,
				tc.composeHelper,
				workspace.UID,
				"db",
			)
			framework.ExpectNoError(err)
			gomega.Expect(dbIDs).To(gomega.HaveLen(1), "db container to be created")
		}, ginkgo.SpecTimeout(framework.GetTimeout()))

		ginkgo.It("specific services", func(ctx context.Context) {
			_, workspace, err := tc.setupAndStartWorkspace(
				ctx,
				"tests/up-docker-compose/testdata/docker-compose-run-services",
			)
			framework.ExpectNoError(err)

			appIDs, err := findComposeContainer(
				ctx,
				tc.dockerHelper,
				tc.composeHelper,
				workspace.UID,
				"app",
			)
			framework.ExpectNoError(err)
			gomega.Expect(appIDs).To(gomega.HaveLen(1), "app container to be created")

			dbIDs, err := findComposeContainer(
				ctx,
				tc.dockerHelper,
				tc.composeHelper,
				workspace.UID,
				"db",
			)
			framework.ExpectNoError(err)
			gomega.Expect(dbIDs).To(gomega.BeEmpty(), "db container not to be created")
		}, ginkgo.SpecTimeout(framework.GetTimeout()))

		ginkgo.It("user lookup with no remoteUser", func(ctx context.Context) {
			_, _, err := tc.setupAndStartWorkspace(
				ctx,
				"tests/up-docker-compose/testdata/docker-compose-lookup-user",
			)
			framework.ExpectNoError(err)
		}, ginkgo.SpecTimeout(framework.GetTimeout()))

		ginkgo.It("dockerfile with args", func(ctx context.Context) {
			tempDir, workspace, err := tc.setupAndStartWorkspace(
				ctx,
				"tests/up-docker-compose/testdata/docker-compose-dockerfile-args",
				"--debug",
			)
			framework.ExpectNoError(err)

			ids, err := findComposeContainer(
				ctx,
				tc.dockerHelper,
				tc.composeHelper,
				workspace.UID,
				"app",
			)
			framework.ExpectNoError(err)
			gomega.Expect(ids).To(gomega.HaveLen(1), "1 compose container to be created")

			buildArgs, err := tc.execSSH(ctx, tempDir, "cat /build-args.txt")
			framework.ExpectNoError(err)
			gomega.Expect(strings.TrimSpace(buildArgs)).
				To(gomega.Equal("mcr.microsoft.com/devcontainers/go:dev-1.24"))
		}, ginkgo.SpecTimeout(framework.GetTimeout()))

		ginkgo.It("multi-stage dockerfile with args", func(ctx context.Context) {
			tempDir, workspace, err := tc.setupAndStartWorkspace(
				ctx,
				"tests/up-docker-compose/testdata/docker-compose-dockerfile-args-multistage",
				"--debug",
			)
			framework.ExpectNoError(err)

			ids, err := findComposeContainer(
				ctx,
				tc.dockerHelper,
				tc.composeHelper,
				workspace.UID,
				"app",
			)
			framework.ExpectNoError(err)
			gomega.Expect(ids).To(gomega.HaveLen(1), "1 compose container to be created")

			buildArgs, err := tc.execSSH(ctx, tempDir, "cat /build-args.txt")
			framework.ExpectNoError(err)
			gomega.Expect(strings.TrimSpace(buildArgs)).
				To(gomega.Equal("mcr.microsoft.com/devcontainers/go:dev-1.24"))
		}, ginkgo.SpecTimeout(framework.GetTimeout()))
	},
)
