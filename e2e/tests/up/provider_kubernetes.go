package up

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/skevetter/devpod/e2e/framework"
)

func waitForPodCount(ctx context.Context, namespace string, expected int, description string) *framework.PodList {
	list := &framework.PodList{}
	gomega.Eventually(func() int {
		cmd := exec.CommandContext(ctx, "kubectl", "get", "pods", "-l", "devpod.sh/created=true", "-o", "json", "-n", namespace)
		stdout, err := cmd.Output()
		if err != nil {
			return -1
		}
		if err := json.Unmarshal(stdout, list); err != nil {
			return -1
		}
		return len(list.Items)
	}, 30*time.Second, 500*time.Millisecond).Should(gomega.Equal(expected), description)
	return list
}

var _ = ginkgo.Describe("testing up command for kubernetes provider", ginkgo.Label("up-provider-kubernetes"), func() {
	var initialDir string

	ginkgo.BeforeEach(func() {
		var err error
		initialDir, err = os.Getwd()
		framework.ExpectNoError(err)
	})

	ginkgo.It("runs devpod in Kubernetes", func(ctx context.Context) {
		f := framework.NewDefaultFramework(initialDir + "/bin")
		tempDir, err := framework.CopyToTempDir("tests/up/testdata/kubernetes")
		framework.ExpectNoError(err)
		ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

		_ = f.DevPodProviderDelete(ctx, "kubernetes")
		err = f.DevPodProviderAdd(ctx, "kubernetes", "-o", "KUBERNETES_NAMESPACE=devpod")
		framework.ExpectNoError(err)
		ginkgo.DeferCleanup(func() {
			err = f.DevPodProviderDelete(context.Background(), "kubernetes")
			framework.ExpectNoError(err)
		})

		// run up
		err = f.DevPodUp(ctx, tempDir)
		framework.ExpectNoError(err)

		// check pod is there
		list := waitForPodCount(ctx, "devpod", 1, "Expect 1 pod")
		framework.ExpectEqual(len(list.Items[0].Spec.Containers), 1, "Expect 1 container")
		framework.ExpectEqual(list.Items[0].Spec.Containers[0].Image, "mcr.microsoft.com/devcontainers/go:1", "Expect container image")

		// check if ssh works
		err = f.DevPodSSHEchoTestString(ctx, tempDir)
		framework.ExpectNoError(err)

		// stop workspace
		err = f.DevPodWorkspaceStop(ctx, tempDir)
		framework.ExpectNoError(err)

		// check pod is gone
		waitForPodCount(ctx, "devpod", 0, "Expect no pods")

		// run up
		err = f.DevPodUp(ctx, tempDir)
		framework.ExpectNoError(err)
		ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, tempDir)

		// check pod is there
		waitForPodCount(ctx, "devpod", 1, "Expect 1 pod")

		// check if ssh works
		err = f.DevPodSSHEchoTestString(ctx, tempDir)
		framework.ExpectNoError(err)
	}, ginkgo.SpecTimeout(framework.GetTimeout()))
})
