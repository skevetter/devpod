package up

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/skevetter/devpod/e2e/framework"
	"github.com/skevetter/devpod/pkg/devcontainer/config"
	docker "github.com/skevetter/devpod/pkg/docker"
	"github.com/skevetter/devpod/pkg/language"
	"github.com/skevetter/log"
)

var _ = ginkgo.Describe("testing up command", ginkgo.Label("up-workspaces"), func() {
	var dockerHelper *docker.DockerHelper
	var initialDir string

	ginkgo.BeforeEach(func() {
		var err error
		initialDir, err = os.Getwd()
		framework.ExpectNoError(err)

		dockerHelper = &docker.DockerHelper{DockerCommand: "docker", Log: log.Default}
	})

	ginkgo.It("with env vars", func(ctx context.Context) {
		f, err := setupDockerProvider(filepath.Join(initialDir, "bin"), "docker")
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
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("create workspace without devcontainer.json", func(ctx context.Context) {
		const providerName = "test-docker"

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
			err = f.DevPodProviderDelete(context.Background(), providerName)
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

		devcontainerPath := path.Join("/workspaces", projectName, ".devcontainer.json")

		containerEnvPath, _, err := f.ExecCommandCapture(ctx, []string{"ssh", "--command", "cat " + devcontainerPath, projectName})
		framework.ExpectNoError(err)
		expectedImageName := language.MapConfig[language.Go].Image

		gomega.Expect(containerEnvPath).To(gomega.Equal(fmt.Sprintf("{\"image\":\"%s\"}", expectedImageName)))

		err = f.DevPodWorkspaceDelete(ctx, tempDir)
		framework.ExpectNoError(err)
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("recreate a local workspace", func(ctx context.Context) {
		const providerName = "test-docker"

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
			err = f.DevPodProviderDelete(context.Background(), providerName)
			framework.ExpectNoError(err)
		})

		err = f.DevPodUp(ctx, tempDir)
		framework.ExpectNoError(err)

		// recreate
		err = f.DevPodUpRecreate(ctx, tempDir)
		framework.ExpectNoError(err)

		err = f.DevPodWorkspaceDelete(ctx, tempDir)
		framework.ExpectNoError(err)
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("recreate a remote workspace", func(ctx context.Context) {
		const providerName = "test-docker"

		f := framework.NewDefaultFramework(initialDir + "/bin")

		// provider add, use and delete afterwards
		err := f.DevPodProviderAdd(ctx, "docker", "--name", providerName)
		framework.ExpectNoError(err)
		err = f.DevPodProviderUse(ctx, providerName)
		framework.ExpectNoError(err)
		ginkgo.DeferCleanup(func() {
			err = f.DevPodProviderDelete(context.Background(), providerName)
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
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("reset a remote workspace", func(ctx context.Context) {
		const providerName = "test-docker"

		f := framework.NewDefaultFramework(initialDir + "/bin")

		// provider add, use and delete afterwards
		err := f.DevPodProviderAdd(ctx, "docker", "--name", providerName)
		framework.ExpectNoError(err)
		err = f.DevPodProviderUse(ctx, providerName)
		framework.ExpectNoError(err)
		ginkgo.DeferCleanup(func() {
			err = f.DevPodProviderDelete(context.Background(), providerName)
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
		err = f.DevPodUpReset(ctx, "https://github.com/loft-sh/examples@subpath:/devpod/jupyter-notebook-hello-world")
		framework.ExpectNoError(err)

		// this should fail! because --reset should trigger a new git clone
		_, err = f.DevPodSSH(ctx, id, fmt.Sprintf("ls /workspaces/%s/DATA", id))
		framework.ExpectError(err)
		// this should fail! because --reset should trigger a new build, so a new rootfs
		_, err = f.DevPodSSH(ctx, id, "ls /ROOTFS")
		framework.ExpectError(err)

		err = f.DevPodWorkspaceDelete(ctx, id)
		framework.ExpectNoError(err)
	}, ginkgo.SpecTimeout(framework.GetTimeout()))
})
