//go:build linux || darwin || unix

package up

import (
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

var _ = ginkgo.Describe("devpod up docker compose test suite", ginkgo.Label("up-docker-compose", "suite"), func() {
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
		tempDir, workspace, err := tc.setupAndStartWorkspace(ctx, "tests/up-docker-compose/testdata/docker-compose-mounts", "--debug")
		framework.ExpectNoError(err)

		ids, err := findComposeContainer(ctx, tc.dockerHelper, tc.composeHelper, workspace.UID, "app")
		framework.ExpectNoError(err)
		gomega.Expect(ids).To(gomega.HaveLen(1), "1 compose container to be created")

		_, _, err = tc.f.ExecCommandCapture(ctx, []string{"ssh", "--command", "touch /home/vscode/mnt1/foo.txt", workspace.ID, "--user", "root"})
		framework.ExpectNoError(err)

		_, _, err = tc.f.ExecCommandCapture(ctx, []string{"ssh", "--command", "echo -n BAR > /home/vscode/mnt1/foo.txt", workspace.ID, "--user", "root"})
		framework.ExpectNoError(err)

		foo, err := tc.execSSH(ctx, tempDir, "cat $HOME/mnt1/foo.txt")
		framework.ExpectNoError(err)
		gomega.Expect(strings.TrimSpace(foo)).To(gomega.Equal("BAR"))

		bar, err := tc.execSSH(ctx, tempDir, "cat $HOME/mnt2/bar.txt")
		framework.ExpectNoError(err)
		gomega.Expect(strings.TrimSpace(bar)).To(gomega.Equal("FOO"))
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("port forwarding", func(ctx context.Context) {
		_, workspace, err := tc.setupAndStartWorkspace(ctx, "tests/up-docker-compose/testdata/docker-compose-forward-ports", "--debug")
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
		tempDir, workspace, err := tc.setupAndStartWorkspace(ctx, "tests/up-docker-compose/testdata/docker-compose-features", "--debug")
		framework.ExpectNoError(err)

		ids, err := findComposeContainer(ctx, tc.dockerHelper, tc.composeHelper, workspace.UID, "app")
		framework.ExpectNoError(err)
		gomega.Expect(ids).To(gomega.HaveLen(1), "1 compose container to be created")

		vclusterVersionOutput, err := tc.execSSH(ctx, tempDir, "vcluster --version")
		framework.ExpectNoError(err)
		gomega.Expect(vclusterVersionOutput).To(gomega.ContainSubstring("vcluster version 0.24.1"))
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("array based commands", func(ctx context.Context) {
		tempDir, err := setupWorkspace("tests/up-docker-compose/testdata/docker-compose-lifecycle-array", tc.initialDir, tc.f)
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

		onCreateCommand, err := tc.execSSH(ctx, tempDir, "cat $HOME/on-create-command.out")
		framework.ExpectNoError(err)
		gomega.Expect(onCreateCommand).To(gomega.ContainSubstring("onCreateCommand"))

		updateContentCommand, err := tc.execSSH(ctx, tempDir, "cat $HOME/update-content-command.out")
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

	ginkgo.It("v2 features", func(ctx context.Context) {
		_, ws, err := tc.setupAndStartWorkspace(ctx, "tests/up-docker-compose/testdata/docker-compose-v2-with-name", "--debug")
		framework.ExpectNoError(err)

		ids, err := findComposeContainer(ctx, tc.dockerHelper, tc.composeHelper, ws.UID, "app")
		framework.ExpectNoError(err)
		gomega.Expect(ids).To(gomega.HaveLen(1), "1 compose container to be created")

		var containerDetails []container.InspectResponse
		err = tc.dockerHelper.Inspect(ctx, ids, "container", &containerDetails)
		framework.ExpectNoError(err)
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("v1 fallback", func(ctx context.Context) {
		_, ws, err := tc.setupAndStartWorkspace(ctx, "tests/up-docker-compose/testdata/docker-compose-v1-fallback", "--debug")
		framework.ExpectNoError(err)

		ids, err := findComposeContainer(ctx, tc.dockerHelper, tc.composeHelper, ws.UID, "app")
		framework.ExpectNoError(err)
		gomega.Expect(ids).To(gomega.HaveLen(1), "1 compose container to be created")

		var containerDetails []container.InspectResponse
		err = tc.dockerHelper.Inspect(ctx, ids, "container", &containerDetails)
		framework.ExpectNoError(err)
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("multiple services", func(ctx context.Context) {
		_, workspace, err := tc.setupAndStartWorkspace(ctx, "tests/up-docker-compose/testdata/docker-compose-multiple-services")
		framework.ExpectNoError(err)

		appIDs, err := findComposeContainer(ctx, tc.dockerHelper, tc.composeHelper, workspace.UID, "app")
		framework.ExpectNoError(err)
		gomega.Expect(appIDs).To(gomega.HaveLen(1), "app container to be created")

		dbIDs, err := findComposeContainer(ctx, tc.dockerHelper, tc.composeHelper, workspace.UID, "db")
		framework.ExpectNoError(err)
		gomega.Expect(dbIDs).To(gomega.HaveLen(1), "db container to be created")
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("specific services", func(ctx context.Context) {
		_, workspace, err := tc.setupAndStartWorkspace(ctx, "tests/up-docker-compose/testdata/docker-compose-run-services")
		framework.ExpectNoError(err)

		appIDs, err := findComposeContainer(ctx, tc.dockerHelper, tc.composeHelper, workspace.UID, "app")
		framework.ExpectNoError(err)
		gomega.Expect(appIDs).To(gomega.HaveLen(1), "app container to be created")

		dbIDs, err := findComposeContainer(ctx, tc.dockerHelper, tc.composeHelper, workspace.UID, "db")
		framework.ExpectNoError(err)
		gomega.Expect(dbIDs).To(gomega.BeEmpty(), "db container not to be created")
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("user lookup with no remoteUser", func(ctx context.Context) {
		_, _, err := tc.setupAndStartWorkspace(ctx, "tests/up-docker-compose/testdata/docker-compose-lookup-user")
		framework.ExpectNoError(err)
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("dockerfile with args", func(ctx context.Context) {
		tempDir, workspace, err := tc.setupAndStartWorkspace(ctx, "tests/up-docker-compose/testdata/docker-compose-dockerfile-args", "--debug")
		framework.ExpectNoError(err)

		ids, err := findComposeContainer(ctx, tc.dockerHelper, tc.composeHelper, workspace.UID, "app")
		framework.ExpectNoError(err)
		gomega.Expect(ids).To(gomega.HaveLen(1), "1 compose container to be created")

		buildArgs, err := tc.execSSH(ctx, tempDir, "cat /build-args.txt")
		framework.ExpectNoError(err)
		gomega.Expect(strings.TrimSpace(buildArgs)).To(gomega.Equal("mcr.microsoft.com/devcontainers/go:dev-1.24"))
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("multi-stage dockerfile with args", func(ctx context.Context) {
		tempDir, workspace, err := tc.setupAndStartWorkspace(ctx, "tests/up-docker-compose/testdata/docker-compose-dockerfile-args-multistage", "--debug")
		framework.ExpectNoError(err)

		ids, err := findComposeContainer(ctx, tc.dockerHelper, tc.composeHelper, workspace.UID, "app")
		framework.ExpectNoError(err)
		gomega.Expect(ids).To(gomega.HaveLen(1), "1 compose container to be created")

		buildArgs, err := tc.execSSH(ctx, tempDir, "cat /build-args.txt")
		framework.ExpectNoError(err)
		gomega.Expect(strings.TrimSpace(buildArgs)).To(gomega.Equal("mcr.microsoft.com/devcontainers/go:dev-1.24"))
	}, ginkgo.SpecTimeout(framework.GetTimeout()))
})
