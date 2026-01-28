package provider

import (
	"context"
	"os"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = framework.DevPodDescribe("devpod provider rename", func() {
	ginkgo.Context("renaming providers", ginkgo.Label("rename"), ginkgo.Ordered, func() {
		ctx := context.Background()
		initialDir, err := os.Getwd()
		if err != nil {
			panic(err)
		}

		// RENAME-1
		ginkgo.It("should rename a provider to a new, valid name", func() {
			tempDir, err := framework.CopyToTempDir("testdata/simple-k8s-provider")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			f := framework.NewDefaultFramework(initialDir + "/bin")

			// Ensure that provider is deleted
			err = f.DevPodProviderDelete(ctx, "provider-rename", "--ignore-not-found")
			framework.ExpectNoError(err)
			err = f.DevPodProviderDelete(ctx, "provider-renamed", "--ignore-not-found")
			framework.ExpectNoError(err)

			// Add provider
			err = f.DevPodProviderAdd(ctx, tempDir+"/provider1.yaml", "--name", "provider-rename")
			framework.ExpectNoError(err)

			// Ensure provider exists
			err = f.DevPodProviderUse(context.Background(), "provider-rename")
			framework.ExpectNoError(err)

			// Rename provider
			err = f.DevPodProviderRename(context.Background(), "provider-rename", "provider-renamed")
			framework.ExpectNoError(err)

			// Ensure old provider is gone and new one exists
			err = f.DevPodProviderUse(context.Background(), "provider-rename")
			framework.ExpectError(err)
			err = f.DevPodProviderUse(context.Background(), "provider-renamed")
			framework.ExpectNoError(err)

			// Cleanup: delete provider
			err = f.DevPodProviderDelete(ctx, "provider-renamed")
			framework.ExpectNoError(err)
		})

		// RENAME-2
		ginkgo.It("should fail to rename a provider to a name that already exists", func() {
			tempDir, err := framework.CopyToTempDir("testdata/simple-k8s-provider")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			f := framework.NewDefaultFramework(initialDir + "/bin")

			// Ensure that providers are deleted
			err = f.DevPodProviderDelete(ctx, "provider-to-rename", "--ignore-not-found")
			framework.ExpectNoError(err)
			err = f.DevPodProviderDelete(ctx, "existing-provider", "--ignore-not-found")
			framework.ExpectNoError(err)

			// Add providers
			err = f.DevPodProviderAdd(ctx, tempDir+"/provider1.yaml", "--name", "provider-to-rename")
			framework.ExpectNoError(err)
			err = f.DevPodProviderAdd(ctx, tempDir+"/provider2.yaml", "--name", "existing-provider")
			framework.ExpectNoError(err)

			// Attempt to rename provider to an existing name
			err = f.DevPodProviderRename(context.Background(), "provider-to-rename", "existing-provider")
			framework.ExpectError(err)

			// Ensure providers still exist
			err = f.DevPodProviderUse(context.Background(), "provider-to-rename")
			framework.ExpectNoError(err)
			err = f.DevPodProviderUse(context.Background(), "existing-provider")
			framework.ExpectNoError(err)

			// Cleanup: delete providers
			err = f.DevPodProviderDelete(ctx, "provider-to-rename")
			framework.ExpectNoError(err)
			err = f.DevPodProviderDelete(ctx, "existing-provider")
			framework.ExpectNoError(err)
		})

		// RENAME-3
		ginkgo.It("should fail to rename a non-existent provider", func() {
			f := framework.NewDefaultFramework(initialDir + "/bin")

			// Ensure that provider is deleted
			err = f.DevPodProviderDelete(ctx, "non-existent-provider", "--ignore-not-found")
			framework.ExpectNoError(err)

			// Attempt to rename non-existent provider
			err = f.DevPodProviderRename(context.Background(), "non-existent-provider", "new-name")
			framework.ExpectError(err)
		})

		// RENAME-4
		ginkgo.It("should rename a provider with an associated workspace", func() {
			tempDir, err := framework.CopyToTempDir("../up/testdata/no-devcontainer")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			f := framework.NewDefaultFramework(initialDir + "/bin")

			providerName := "provider-with-workspace"
			renamedProviderName := "renamed-provider-with-workspace"

			// Ensure that providers are deleted
			err = f.DevPodProviderDelete(ctx, providerName, "--ignore-not-found")
			framework.ExpectNoError(err)
			err = f.DevPodProviderDelete(ctx, renamedProviderName, "--ignore-not-found")
			framework.ExpectNoError(err)

			// Add and use provider
			err = f.DevPodProviderAdd(ctx, "docker", "--name", providerName)
			framework.ExpectNoError(err)
			err = f.DevPodProviderUse(ctx, providerName)
			framework.ExpectNoError(err)

			// Create workspace
			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			// Rename provider
			err = f.DevPodProviderRename(ctx, providerName, renamedProviderName)
			framework.ExpectNoError(err)

			// Verify that the old provider is gone and the new one exists
			err = f.DevPodProviderUse(ctx, providerName)
			framework.ExpectError(err)
			err = f.DevPodProviderUse(ctx, renamedProviderName)
			framework.ExpectNoError(err)

			// Verify that the workspace is now accessible
			_, err = f.DevPodSSH(ctx, tempDir, "echo 'hello'")
			framework.ExpectNoError(err)

			// Cleanup
			err = f.DevPodWorkspaceDelete(ctx, tempDir)
			framework.ExpectNoError(err)
			err = f.DevPodProviderDelete(ctx, renamedProviderName)
			framework.ExpectNoError(err)
		})

		// RENAME-5
		ginkgo.It("should fail to rename a provider to an invalid name", func() {
			tempDir, err := framework.CopyToTempDir("testdata/simple-k8s-provider")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			f := framework.NewDefaultFramework(initialDir + "/bin")

			providerName := "provider-to-rename-invalid"

			// Ensure that provider is deleted
			err = f.DevPodProviderDelete(ctx, providerName, "--ignore-not-found")
			framework.ExpectNoError(err)

			// Add provider
			err = f.DevPodProviderAdd(ctx, tempDir+"/provider1.yaml", "--name", providerName)
			framework.ExpectNoError(err)

			// Attempt to rename provider to an invalid name
			err = f.DevPodProviderRename(context.Background(), providerName, "invalid/name")
			framework.ExpectError(err)

			// Ensure provider still exists
			err = f.DevPodProviderUse(context.Background(), providerName)
			framework.ExpectNoError(err)

			// Cleanup: delete provider
			err = f.DevPodProviderDelete(ctx, providerName)
			framework.ExpectNoError(err)
		})
	})
})
