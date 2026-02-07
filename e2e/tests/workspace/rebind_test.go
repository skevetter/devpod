package workspace

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/skevetter/devpod/e2e/framework"
	"github.com/skevetter/devpod/pkg/workspace"
)

var _ = ginkgo.Describe("[workspace] devpod workspace rebind", ginkgo.Label("rebind"), ginkgo.Ordered, func() {
	ctx := context.Background()
	initialDir, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	ginkgo.It("should rebind a workspace to a new provider", func() {
		tempDir, err := framework.CopyToTempDir("../up/testdata/no-devcontainer")
		framework.ExpectNoError(err)
		ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

		f := framework.NewDefaultFramework(initialDir + "/bin")

		provider1Name := "provider1-for-rebind"
		provider2Name := "provider2-for-rebind"

		// Ensure that providers are deleted.
		err = f.DevPodProviderDelete(ctx, provider1Name, "--ignore-not-found")
		framework.ExpectNoError(err)
		err = f.DevPodProviderDelete(ctx, provider2Name, "--ignore-not-found")
		framework.ExpectNoError(err)

		// Add and use first provider.
		err = f.DevPodProviderAdd(ctx, "docker", "--name", provider1Name)
		framework.ExpectNoError(err)
		err = f.DevPodProviderUse(ctx, provider1Name)
		framework.ExpectNoError(err)

		// Add second provider.
		err = f.DevPodProviderAdd(ctx, "docker", "--name", provider2Name)
		framework.ExpectNoError(err)

		// Create workspace.
		err = f.DevPodUp(ctx, tempDir)
		framework.ExpectNoError(err)

		// Normalize workspace ID using same method as DevPod internals.
		workspaceID := workspace.ToID(filepath.Base(tempDir))

		// Wait for the workspace to reach running state.
		gomega.Eventually(func() string {
			status, err := f.DevPodStatus(ctx, workspaceID)
			if err != nil {
				return "error"
			}
			state := string(status.State)
			return state
		}).WithTimeout(30 * time.Second).
			WithPolling(1 * time.Second).
			Should(gomega.Equal("Running"))

		// Stop the workspace before rebinding (required by the enhanced functionality).
		err = f.DevPodStop(ctx, workspaceID)
		framework.ExpectNoError(err)

		// Wait for the workspace to reach stopped or not found state.
		gomega.Eventually(func() string {
			status, err := f.DevPodStatus(ctx, workspaceID)
			if err != nil {
				return "error"
			}
			state := string(status.State)
			return state
		}).WithTimeout(30 * time.Second).
			WithPolling(1 * time.Second).
			Should(gomega.Or(gomega.Equal("Stopped"), gomega.Equal("NotFound")))

		err = f.DevPodWorkspaceRebind(ctx, workspaceID, provider2Name)
		framework.ExpectNoError(err)

		// Verify that the workspace is now associated with the second provider.
		// We can do this by trying to access it after switching to the second provider.
		err = f.DevPodProviderUse(ctx, provider2Name)
		framework.ExpectNoError(err)

		// Start the workspace with the new provider and test it.
		err = f.DevPodUp(ctx, tempDir)
		framework.ExpectNoError(err)

		_, err = f.DevPodSSH(ctx, workspaceID, "echo 'hello'")
		framework.ExpectNoError(err)

		// Cleanup.
		err = f.DevPodWorkspaceDelete(ctx, tempDir)
		framework.ExpectNoError(err)
		err = f.DevPodProviderDelete(ctx, provider1Name)
		framework.ExpectNoError(err)
		err = f.DevPodProviderDelete(ctx, provider2Name)
		framework.ExpectNoError(err)
	})

	ginkgo.It("should fail to rebind a running workspace", func() {
		tempDir, err := framework.CopyToTempDir("../up/testdata/no-devcontainer")
		framework.ExpectNoError(err)
		ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

		f := framework.NewDefaultFramework(initialDir + "/bin")

		provider1Name := "provider1-running-test"
		provider2Name := "provider2-running-test"

		// Ensure that providers are deleted.
		err = f.DevPodProviderDelete(ctx, provider1Name, "--ignore-not-found")
		framework.ExpectNoError(err)
		err = f.DevPodProviderDelete(ctx, provider2Name, "--ignore-not-found")
		framework.ExpectNoError(err)

		// Add and use first provider.
		err = f.DevPodProviderAdd(ctx, "docker", "--name", provider1Name)
		framework.ExpectNoError(err)
		err = f.DevPodProviderUse(ctx, provider1Name)
		framework.ExpectNoError(err)

		// Add second provider.
		err = f.DevPodProviderAdd(ctx, "docker", "--name", provider2Name)
		framework.ExpectNoError(err)

		// Create and start workspace.
		err = f.DevPodUp(ctx, tempDir)
		framework.ExpectNoError(err)

		// Normalize workspace ID using same method as DevPod internals.
		workspaceID := workspace.ToID(filepath.Base(tempDir))

		// Try to rebind the running workspace - this should fail with the new implementation.
		err = f.DevPodWorkspaceRebind(ctx, workspaceID, provider2Name)
		framework.ExpectError(err)

		// Cleanup.
		err = f.DevPodWorkspaceDelete(ctx, tempDir)
		framework.ExpectNoError(err)
		err = f.DevPodProviderDelete(ctx, provider1Name)
		framework.ExpectNoError(err)
		err = f.DevPodProviderDelete(ctx, provider2Name)
		framework.ExpectNoError(err)
	})
})
