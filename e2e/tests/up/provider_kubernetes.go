package up

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = ginkgo.Describe("testing up command for kubernetes provider", ginkgo.Label("up-provider-kubernetes"), func() {
	var initialDir string

	ginkgo.BeforeEach(func() {
		var err error
		initialDir, err = os.Getwd()
		framework.ExpectNoError(err)
	})

	ginkgo.It("runs devpod in Kubernetes", func() {
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
		cmd := exec.CommandContext(ctx, "kubectl", "get", "pods", "-l", "devpod.sh/created=true", "-o", "json", "-n", "devpod")
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
		cmd = exec.CommandContext(ctx, "kubectl", "get", "pods", "-l", "devpod.sh/created=true", "-o", "json", "-n", "devpod")
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
		cmd = exec.CommandContext(ctx, "kubectl", "get", "pods", "-l", "devpod.sh/created=true", "-o", "json", "-n", "devpod")
		stdout, err = cmd.Output()
		framework.ExpectNoError(err)
		ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, tempDir)

		// check if pod is there
		list = &framework.PodList{}
		err = json.Unmarshal(stdout, list)
		framework.ExpectNoError(err)
		framework.ExpectEqual(len(list.Items), 1, "Expect 1 pod")

		// check if ssh works
		err = f.DevPodSSHEchoTestString(ctx, tempDir)
		framework.ExpectNoError(err)
	})
})
