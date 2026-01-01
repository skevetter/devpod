package up

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/skevetter/devpod/e2e/framework"
	"github.com/skevetter/devpod/pkg/devcontainer/config"
	docker "github.com/skevetter/devpod/pkg/docker"
	"github.com/skevetter/devpod/pkg/language"
	"github.com/skevetter/log"
)

var _ = DevPodDescribe("devpod up test suite", func() {
	ginkgo.Context("testing up command", ginkgo.Label("up"), func() {
		var dockerHelper *docker.DockerHelper
		var initialDir string

		ginkgo.BeforeEach(func() {
			var err error
			initialDir, err = os.Getwd()
			framework.ExpectNoError(err)

			dockerHelper = &docker.DockerHelper{DockerCommand: "docker", Log: log.Default}
			framework.ExpectNoError(err)
		})

		ginkgo.It("with env vars", ginkgo.Label("workspace", "env"), func() {
			ctx := context.Background()
			f, err := setupDockerProvider(initialDir+"/bin", "docker")
			framework.ExpectNoError(err)

			name := "vscode-remote-try-python"
			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), name)

			// Wait for devpod workspace to come online (deadline: 30s)
			err = f.DevPodUp(ctx, "https://github.com/microsoft/vscode-remote-try-python.git")
			framework.ExpectNoError(err)

			// check env var
			out, err := f.DevPodSSH(ctx, name, "echo -n $TEST_VAR")
			framework.ExpectNoError(err)
			framework.ExpectEqual(out, "", "should be empty")

			// set env var
			value := "test-variable"
			err = f.DevPodUp(ctx, name, "--workspace-env", "TEST_VAR="+value)
			framework.ExpectNoError(err)

			// check env var
			out, err = f.DevPodSSH(ctx, name, "echo -n $TEST_VAR")
			framework.ExpectNoError(err)
			framework.ExpectEqual(out, value, "should be set now")

			// check env var again
			err = f.DevPodUp(ctx, name)
			framework.ExpectNoError(err)

			// check env var
			out, err = f.DevPodSSH(ctx, name, "echo -n $TEST_VAR")
			framework.ExpectNoError(err)
			framework.ExpectEqual(out, value, "should still be set")

			// delete env var
			err = f.DevPodUp(ctx, name, "--workspace-env", "TEST_VAR=")
			framework.ExpectNoError(err)

			// check env var
			out, err = f.DevPodSSH(ctx, name, "echo -n $TEST_VAR")
			framework.ExpectNoError(err)
			framework.ExpectEqual(out, "", "should be empty")

			// set env vars with file
			tmpDir, err := framework.CreateTempDir()
			framework.ExpectNoError(err)

			// create invalid env file
			invalidData := []byte("TEST VAR=" + value)
			workspaceEnvFileInvalid := filepath.Join(tmpDir, ".invalid")
			err = os.WriteFile(
				workspaceEnvFileInvalid,
				invalidData, 0o644)
			framework.ExpectNoError(err)
			defer func() { _ = os.Remove(workspaceEnvFileInvalid) }()

			// set env var
			err = f.DevPodUp(ctx, name, "--workspace-env-file", workspaceEnvFileInvalid)
			framework.ExpectError(err)

			// create valid env file
			validData := []byte("TEST_VAR=" + value)
			workspaceEnvFileValid := filepath.Join(tmpDir, ".valid")
			err = os.WriteFile(
				workspaceEnvFileValid,
				validData, 0o644)
			framework.ExpectNoError(err)
			defer func() { _ = os.Remove(workspaceEnvFileValid) }()

			// set env var
			err = f.DevPodUp(ctx, name, "--workspace-env-file", workspaceEnvFileValid)
			framework.ExpectNoError(err)

			// check env var
			out, err = f.DevPodSSH(ctx, name, "echo -n $TEST_VAR")
			framework.ExpectNoError(err)
			framework.ExpectEqual(out, value, "should be set now")

			// delete env var
			err = f.DevPodUp(ctx, name, "--workspace-env", "TEST_VAR=")
			framework.ExpectNoError(err)

			// check env var
			out, err = f.DevPodSSH(ctx, name, "echo -n $TEST_VAR")
			framework.ExpectNoError(err)
			framework.ExpectEqual(out, "", "should be empty")

			// create a second valid env file with a different env var
			validData = []byte("TEST_OTHER_VAR=" + value)
			workspaceEnvFileValid2 := filepath.Join(tmpDir, ".valid2")
			err = os.WriteFile(
				workspaceEnvFileValid2,
				validData, 0o644)
			framework.ExpectNoError(err)
			defer func() { _ = os.Remove(workspaceEnvFileValid2) }()

			// set env var from both files
			err = f.DevPodUp(ctx, name, "--workspace-env-file", fmt.Sprintf("%s,%s", workspaceEnvFileValid, workspaceEnvFileValid2))
			framework.ExpectNoError(err)

			// check env var from .valid file
			out, err = f.DevPodSSH(ctx, name, "echo -n $TEST_VAR")
			framework.ExpectNoError(err)
			framework.ExpectEqual(out, value, "should be set now")

			// check env var from .valid2 file
			out, err = f.DevPodSSH(ctx, name, "echo -n $TEST_OTHER_VAR")
			framework.ExpectNoError(err)
			framework.ExpectEqual(out, value, "should be set now")
		})

		ginkgo.It("should allow checkout of a GitRepo from a commit hash", ginkgo.Label("git", "commit"), func() {
			ctx := context.Background()
			f, err := setupDockerProvider(initialDir+"/bin", "docker")
			framework.ExpectNoError(err)

			name := "sha256-0c1547c"
			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), name)

			// Wait for devpod workspace to come online (deadline: 30s)
			err = f.DevPodUp(ctx, "github.com/microsoft/vscode-remote-try-python@sha256:0c1547c")
			framework.ExpectNoError(err)
		})

		ginkgo.It("should allow checkout of a GitRepo from a pull request reference", ginkgo.Label("git", "pr"), func() {
			ctx := context.Background()
			f, err := setupDockerProvider(initialDir+"/bin", "docker")
			framework.ExpectNoError(err)

			name := "pr100"
			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), name)

			// Wait for devpod workspace to come online (deadline: 30s)
			err = f.DevPodUp(ctx, "github.com/skevetter/devpod@pull/100/head")
			framework.ExpectNoError(err)
		})

		ginkgo.It("run devpod in Kubernetes", ginkgo.Label("provider-kubernetes"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/bin")
			tempDir, err := framework.CopyToTempDir("tests/up/testdata/kubernetes")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			_ = f.DevPodProviderDelete(ctx, "kubernetes")
			err = f.DevPodProviderAdd(ctx, "kubernetes", "-o", "KUBERNETES_NAMESPACE=devpod")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(func() {
				err = f.DevPodProviderDelete(ctx, "kubernetes")
				framework.ExpectNoError(err)
			})

			// run up
			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			// check pod is there
			cmd := exec.Command("kubectl", "get", "pods", "-l", "devpod.sh/created=true", "-o", "json", "-n", "devpod")
			stdout, err := cmd.Output()
			framework.ExpectNoError(err)

			// check if pod is there
			list := &framework.PodList{}
			err = json.Unmarshal(stdout, list)
			framework.ExpectNoError(err)
			framework.ExpectEqual(len(list.Items), 1, "Expect 1 pod")
			framework.ExpectEqual(len(list.Items[0].Spec.Containers), 1, "Expect 1 container")
			framework.ExpectEqual(list.Items[0].Spec.Containers[0].Image, "mcr.microsoft.com/devcontainers/go:1", "Expect container image")

			// check if ssh works
			err = f.DevPodSSHEchoTestString(ctx, tempDir)
			framework.ExpectNoError(err)

			// stop workspace
			err = f.DevPodWorkspaceStop(ctx, tempDir)
			framework.ExpectNoError(err)

			// check pod is there
			cmd = exec.Command("kubectl", "get", "pods", "-l", "devpod.sh/created=true", "-o", "json", "-n", "devpod")
			stdout, err = cmd.Output()
			framework.ExpectNoError(err)

			// check if pod is there
			list = &framework.PodList{}
			err = json.Unmarshal(stdout, list)
			framework.ExpectNoError(err)
			framework.ExpectEqual(len(list.Items), 0, "Expect no pods")

			// run up
			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			// check pod is there
			cmd = exec.Command("kubectl", "get", "pods", "-l", "devpod.sh/created=true", "-o", "json", "-n", "devpod")
			stdout, err = cmd.Output()
			framework.ExpectNoError(err)

			// check if pod is there
			list = &framework.PodList{}
			err = json.Unmarshal(stdout, list)
			framework.ExpectNoError(err)
			framework.ExpectEqual(len(list.Items), 1, "Expect 1 pod")

			// check if ssh works
			err = f.DevPodSSHEchoTestString(ctx, tempDir)
			framework.ExpectNoError(err)

			// delete workspace
			err = f.DevPodWorkspaceDelete(ctx, tempDir)
			framework.ExpectNoError(err)
		})

		ginkgo.It("create workspace without devcontainer.json", ginkgo.Label("workspace", "no-devcontainer"), func() {
			const providerName = "test-docker"
			ctx := context.Background()

			f := framework.NewDefaultFramework(initialDir + "/bin")
			tempDir, err := framework.CopyToTempDir("tests/up/testdata/no-devcontainer")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			// provider add, use and delete afterwards
			err = f.DevPodProviderAdd(ctx, "docker", "--name", providerName)
			framework.ExpectNoError(err)
			err = f.DevPodProviderUse(ctx, providerName)
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(func() {
				_ = f.DevPodWorkspaceDelete(ctx, tempDir)
				err = f.DevPodProviderDelete(ctx, providerName)
				framework.ExpectNoError(err)
			})

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

			devcontainerPath := filepath.Join("/workspaces", projectName, ".devcontainer.json")

			containerEnvPath, _, err := f.ExecCommandCapture(ctx, []string{"ssh", "--command", "cat " + devcontainerPath, projectName})
			framework.ExpectNoError(err)
			expectedImageName := language.MapConfig[language.Go].Image

			gomega.Expect(containerEnvPath).To(gomega.Equal(fmt.Sprintf("{\"image\":\"%s\"}", expectedImageName)))

			err = f.DevPodWorkspaceDelete(ctx, tempDir)
			framework.ExpectNoError(err)
		})

		ginkgo.It("recreate a local workspace", ginkgo.Label("workspace", "recreate"), func() {
			const providerName = "test-docker"
			ctx := context.Background()

			f := framework.NewDefaultFramework(initialDir + "/bin")
			tempDir, err := framework.CopyToTempDir("tests/up/testdata/no-devcontainer")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			// provider add, use and delete afterwards
			err = f.DevPodProviderAdd(ctx, "docker", "--name", providerName)
			framework.ExpectNoError(err)
			err = f.DevPodProviderUse(ctx, providerName)
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(func() {
				err = f.DevPodProviderDelete(ctx, providerName)
				framework.ExpectNoError(err)
			})

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			// recreate
			err = f.DevPodUpRecreate(ctx, tempDir)
			framework.ExpectNoError(err)

			err = f.DevPodWorkspaceDelete(ctx, tempDir)
			framework.ExpectNoError(err)
		})

		ginkgo.It("create workspace in a subpath", ginkgo.Label("git", "subpath"), func() {
			const providerName = "test-docker"
			ctx := context.Background()

			f := framework.NewDefaultFramework(initialDir + "/bin")

			// provider add, use and delete afterwards
			err := f.DevPodProviderAdd(ctx, "docker", "--name", providerName)
			framework.ExpectNoError(err)
			err = f.DevPodProviderUse(ctx, providerName)
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(func() {
				err = f.DevPodProviderDelete(ctx, providerName)
				framework.ExpectNoError(err)
			})

			err = f.DevPodUp(ctx, "https://github.com/loft-sh/examples@subpath:/devpod/jupyter-notebook-hello-world")
			framework.ExpectNoError(err)

			id := "subpath--devpod-jupyter-notebook-hello-world"
			out, err := f.DevPodSSH(ctx, id, "pwd")
			framework.ExpectNoError(err)
			framework.ExpectEqual(out, fmt.Sprintf("/workspaces/%s\n", id), "should be subpath")

			err = f.DevPodWorkspaceDelete(ctx, id)
			framework.ExpectNoError(err)
		})

		ginkgo.It("recreate a remote workspace", ginkgo.Label("workspace", "recreate"), func() {
			const providerName = "test-docker"
			ctx := context.Background()

			f := framework.NewDefaultFramework(initialDir + "/bin")

			// provider add, use and delete afterwards
			err := f.DevPodProviderAdd(ctx, "docker", "--name", providerName)
			framework.ExpectNoError(err)
			err = f.DevPodProviderUse(ctx, providerName)
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(func() {
				err = f.DevPodProviderDelete(ctx, providerName)
				framework.ExpectNoError(err)
			})

			id := "subpath--devpod-jupyter-notebook-hello-world"
			err = f.DevPodUp(ctx, "https://github.com/loft-sh/examples@subpath:/devpod/jupyter-notebook-hello-world")
			framework.ExpectNoError(err)

			_, err = f.DevPodSSH(ctx, id, "pwd")
			framework.ExpectNoError(err)

			// recreate
			err = f.DevPodUpRecreate(ctx, "https://github.com/loft-sh/examples@subpath:/devpod/jupyter-notebook-hello-world")
			framework.ExpectNoError(err)

			_, err = f.DevPodSSH(ctx, id, "pwd")
			framework.ExpectNoError(err)

			err = f.DevPodWorkspaceDelete(ctx, id)
			framework.ExpectNoError(err)
		})

		ginkgo.It("reset a remote workspace", ginkgo.Label("workspace", "reset"), func() {
			const providerName = "test-docker"
			ctx := context.Background()

			f := framework.NewDefaultFramework(initialDir + "/bin")

			// provider add, use and delete afterwards
			err := f.DevPodProviderAdd(ctx, "docker", "--name", providerName)
			framework.ExpectNoError(err)
			err = f.DevPodProviderUse(ctx, providerName)
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(func() {
				err = f.DevPodWorkspaceDelete(ctx, "jupyter-notebook-hello-world")
				framework.ExpectNoError(err)
				err = f.DevPodProviderDelete(ctx, providerName)
				framework.ExpectNoError(err)
			})

			id := "subpath--devpod-jupyter-notebook-hello-world"
			err = f.DevPodUp(ctx, "https://github.com/loft-sh/examples@subpath:/devpod/jupyter-notebook-hello-world")
			framework.ExpectNoError(err)

			// create files in root and in workspace, after create we expect data to still be there
			_, err = f.DevPodSSH(ctx, id, fmt.Sprintf("sudo touch /workspaces/%s/DATA", id))
			framework.ExpectNoError(err)
			_, err = f.DevPodSSH(ctx, id, "sudo touch /ROOTFS")
			framework.ExpectNoError(err)

			// reset
			err = f.DevPodUpReset(ctx, "https://github.com/loft-sh/examples/@subpath:/devpod/jupyter-notebook-hello-world")
			framework.ExpectNoError(err)

			// this should fail! because --reset should trigger a new git clone
			_, err = f.DevPodSSH(ctx, id, fmt.Sprintf("ls /workspaces/%s/DATA", id))
			framework.ExpectError(err)
			// this should fail! because --recreare should trigger a new build, so a new rootfs
			_, err = f.DevPodSSH(ctx, id, "ls /ROOTFS")
			framework.ExpectError(err)

			err = f.DevPodWorkspaceDelete(ctx, id)
			framework.ExpectNoError(err)
		})

		ginkgo.Context("print error message correctly", func() {
			ginkgo.It("make sure devpod output is correct and log-output works correctly", ginkgo.Label("error", "output"), func(ctx context.Context) {
				f := framework.NewDefaultFramework(initialDir + "/bin")
				tempDir, err := framework.CopyToTempDir("tests/up/testdata/docker")
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

				err = f.DevPodProviderAdd(ctx, "docker", "--name", "test-docker")
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(func() {
					err = f.DevPodProviderDelete(context.Background(), "test-docker")
					framework.ExpectNoError(err)
				})

				err = f.DevPodProviderUse(ctx, "test-docker", "-o", "DOCKER_PATH=abc", "--skip-init")
				framework.ExpectNoError(err)

				// Wait for devpod workspace to come online
				stdout, stderr, err := f.DevPodUpStreams(ctx, tempDir, "--log-output=json")
				deleteErr := f.DevPodWorkspaceDelete(ctx, tempDir, "--force")
				framework.ExpectNoError(deleteErr)
				framework.ExpectError(err, "expected error")
				framework.ExpectNoError(verifyLogStream(strings.NewReader(stdout)))
				framework.ExpectNoError(verifyLogStream(strings.NewReader(stderr)))
				framework.ExpectNoError(findMessage(strings.NewReader(stderr), "exec: \"abc\": executable file not found in $PATH"))
			}, ginkgo.SpecTimeout(framework.GetTimeout()))
		})

		ginkgo.Context("cleanup up on failure", func() {
			ginkgo.It("ensure workspace cleanup when failing to create a workspace", ginkgo.Label("error", "cleanup"), func(ctx context.Context) {
				f, err := setupDockerProvider(initialDir+"/bin", "docker")
				framework.ExpectNoError(err)

				initialList, err := f.DevPodList(ctx)
				framework.ExpectNoError(err)
				// Wait for devpod workspace to come online (deadline: 30s)
				err = f.DevPodUp(ctx, "github.com/i/do-not-exist.git")
				framework.ExpectError(err)

				out, err := f.DevPodList(ctx)
				framework.ExpectNoError(err)
				framework.ExpectEqual(out, initialList)
			}, ginkgo.SpecTimeout(framework.GetTimeout()))

			ginkgo.It("should fail with error when bind mount source does not exist", ginkgo.Label("error", "bind-mount"), func(ctx context.Context) {
				f, err := setupDockerProvider(initialDir+"/bin", "docker")
				framework.ExpectNoError(err)

				tempDir, err := framework.CopyToTempDir("tests/up/testdata/docker-invalid-bind-mount")
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

				err = f.DevPodUp(ctx, tempDir, "--debug")

				gomega.Expect(err).To(gomega.HaveOccurred())
				gomega.Expect(err.Error()).To(gomega.ContainSubstring("devpod up failed"))
				gomega.Expect(err.Error()).To(gomega.ContainSubstring("exit status 1"))
			}, ginkgo.SpecTimeout(framework.GetTimeout()))

			ginkgo.It("ensure workspace cleanup when not a git or folder", ginkgo.Label("error", "cleanup"), func(ctx context.Context) {
				f, err := setupDockerProvider(initialDir+"/bin", "docker")
				framework.ExpectNoError(err)

				initialList, err := f.DevPodList(ctx)
				framework.ExpectNoError(err)
				// Wait for devpod workspace to come online (deadline: 30s)
				err = f.DevPodUp(ctx, "notfound.loft.sh")
				framework.ExpectError(err)

				out, err := f.DevPodList(ctx)
				framework.ExpectNoError(err)
				framework.ExpectEqual(out, initialList)
			}, ginkgo.SpecTimeout(framework.GetTimeout()))
		})

		ginkgo.Context("devcontainer features dependsOn", func() {
			ginkgo.It("should automatically install dependsOn features", ginkgo.Label("features", "depends-on"), func(ctx context.Context) {
				f, err := setupDockerProvider(initialDir+"/bin", "docker")
				framework.ExpectNoError(err)

				tempDir, err := framework.CopyToTempDir("tests/up/testdata/docker-features-depends-on")
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

				workspaceName := filepath.Base(tempDir)
				ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), workspaceName)

				// This should now succeed with dependsOn implementation
				err = f.DevPodUp(ctx, tempDir)
				framework.ExpectNoError(err)

				// Test if the dependency (hello command) is available
				out, err := f.DevPodSSH(ctx, workspaceName, "test-depends-on")
				framework.ExpectNoError(err)
				// The output contains ANSI color codes, so only check for the text
				gomega.Expect(out).To(gomega.ContainSubstring("SUCCESS: hello command is available"))
				gomega.Expect(out).To(gomega.ContainSubstring("hey, vscode"))
			}, ginkgo.SpecTimeout(framework.GetTimeout()))

			ginkgo.It("should handle nested dependencies", ginkgo.Label("features", "depends-on"), func(ctx context.Context) {
				f, err := setupDockerProvider(initialDir+"/bin", "docker")
				framework.ExpectNoError(err)

				tempDir, err := framework.CopyToTempDir("tests/up/testdata/docker-features-nested-depends-on")
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

				workspaceName := filepath.Base(tempDir)
				ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), workspaceName)

				err = f.DevPodUp(ctx, tempDir)
				framework.ExpectNoError(err)

				// Test nested dependency chain works
				out, err := f.DevPodSSH(ctx, workspaceName, "test-nested-chain")
				framework.ExpectNoError(err)
				gomega.Expect(out).To(gomega.ContainSubstring("All dependencies available"))
			}, ginkgo.SpecTimeout(framework.GetTimeout()))

			ginkgo.It("should detect circular dependencies", ginkgo.Label("features", "depends-on"), func(ctx context.Context) {
				f, err := setupDockerProvider(initialDir+"/bin", "docker")
				framework.ExpectNoError(err)

				tempDir, err := framework.CopyToTempDir("tests/up/testdata/docker-features-circular-depends-on")
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

				workspaceName := filepath.Base(tempDir)
				ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), workspaceName)

				// This should fail with circular dependency error
				err = f.DevPodUp(ctx, tempDir)
				// The logs show "circular dependency detected" in the debug output
				framework.ExpectError(err)
			}, ginkgo.SpecTimeout(framework.GetTimeout()))

			ginkgo.It("should handle dependsOn with options", ginkgo.Label("features", "depends-on"), func(ctx context.Context) {
				f, err := setupDockerProvider(initialDir+"/bin", "docker")
				framework.ExpectNoError(err)

				tempDir, err := framework.CopyToTempDir("tests/up/testdata/docker-features-depends-on-options")
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

				workspaceName := filepath.Base(tempDir)
				ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), workspaceName)

				err = f.DevPodUp(ctx, tempDir)
				framework.ExpectNoError(err)

				// Test dependency installed with correct options
				out, err := f.DevPodSSH(ctx, workspaceName, "hello")
				framework.ExpectNoError(err)
				gomega.Expect(out).To(gomega.ContainSubstring("custom greeting"))
			}, ginkgo.SpecTimeout(framework.GetTimeout()))

			ginkgo.It("should handle mixed dependsOn and installsAfter", ginkgo.Label("features", "mixed"), func(ctx context.Context) {
				f, err := setupDockerProvider(initialDir+"/bin", "docker")
				framework.ExpectNoError(err)

				tempDir, err := framework.CopyToTempDir("tests/up/testdata/docker-features-mixed-dependencies")
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

				workspaceName := filepath.Base(tempDir)
				ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), workspaceName)

				err = f.DevPodUp(ctx, tempDir)
				framework.ExpectNoError(err)

				// Test correct installation order
				out, err := f.DevPodSSH(ctx, workspaceName, "test-install-order")
				framework.ExpectNoError(err)
				gomega.Expect(out).To(gomega.ContainSubstring("Correct order"))
			}, ginkgo.SpecTimeout(framework.GetTimeout()))

			ginkgo.It("should detect self-dependency", ginkgo.Label("features", "depends-on"), func(ctx context.Context) {
				f, err := setupDockerProvider(initialDir+"/bin", "docker")
				framework.ExpectNoError(err)

				tempDir, err := framework.CopyToTempDir("tests/up/testdata/docker-features-self-dependency")
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

				workspaceName := filepath.Base(tempDir)
				ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), workspaceName)

				// Should fail with circular dependency error
				err = f.DevPodUp(ctx, tempDir)
				framework.ExpectError(err)
			}, ginkgo.SpecTimeout(framework.GetTimeout()))

			ginkgo.It("should handle non-existent dependency gracefully", ginkgo.Label("features", "depends-on"), func(ctx context.Context) {
				f, err := setupDockerProvider(initialDir+"/bin", "docker")
				framework.ExpectNoError(err)

				tempDir, err := framework.CopyToTempDir("tests/up/testdata/docker-features-nonexistent-dependency")
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

				workspaceName := filepath.Base(tempDir)
				ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), workspaceName)

				// Should fail when dependency cannot be resolved
				err = f.DevPodUp(ctx, tempDir)
				framework.ExpectError(err)
			}, ginkgo.SpecTimeout(framework.GetTimeout()))

			ginkgo.It("should handle shared dependencies correctly", ginkgo.Label("features", "depends-on"), func(ctx context.Context) {
				f, err := setupDockerProvider(initialDir+"/bin", "docker")
				framework.ExpectNoError(err)

				tempDir, err := framework.CopyToTempDir("tests/up/testdata/docker-features-shared-dependency")
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

				workspaceName := filepath.Base(tempDir)
				ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), workspaceName)

				err = f.DevPodUp(ctx, tempDir)
				framework.ExpectNoError(err)

				// Verify shared dependency was installed only once and both features work
				out, err := f.DevPodSSH(ctx, workspaceName, "hello")
				framework.ExpectNoError(err)
				// Should contain greeting from one of the features (last one wins)
				gomega.Expect(out).To(gomega.ContainSubstring("from"))
			}, ginkgo.SpecTimeout(framework.GetTimeout()))
		})

		ginkgo.It("should handle forward reference dependencies", ginkgo.Label("features", "depends-on"), func(ctx context.Context) {
			f, err := setupDockerProvider(initialDir+"/bin", "docker")
			framework.ExpectNoError(err)

			tempDir, err := framework.CopyToTempDir("tests/up/testdata/docker-features-forward-reference")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			workspaceName := filepath.Base(tempDir)
			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), workspaceName)

			// This should not fail with "Parent does not exist" error
			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			// Test that both features are installed correctly
			out, err := f.DevPodSSH(ctx, workspaceName, "python3 --version")
			framework.ExpectNoError(err)
			gomega.Expect(out).To(gomega.ContainSubstring("Python 3.11"))
		}, ginkgo.SpecTimeout(framework.GetTimeout()*5)) // This test compiles Python
	})
})
