package provider

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/skevetter/devpod/e2e/framework"
	"github.com/skevetter/devpod/pkg/workspace"
)

var _ = DevPodDescribe("devpod provider test suite", func() {
	ginkgo.Context("testing non-machine providers", ginkgo.Label("provider"), ginkgo.Ordered, func() {
		ctx := context.Background()
		initialDir, err := os.Getwd()
		if err != nil {
			panic(err)
		}

		ginkgo.It("should add simple provider and delete it", func() {
			tempDir, err := framework.CopyToTempDir("tests/provider/testdata/simple-k8s-provider")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			f := framework.NewDefaultFramework(initialDir + "/bin")

			// Ensure that provider 1 is deleted
			err = f.DevPodProviderDelete(ctx, "provider1", "--ignore-not-found")
			framework.ExpectNoError(err)

			// Add provider 1
			err = f.DevPodProviderAdd(ctx, tempDir+"/provider1.yaml")
			framework.ExpectNoError(err)

			// Ensure provider 1 exists but not provider X
			err = f.DevPodProviderUse(context.Background(), "provider1")
			framework.ExpectNoError(err)
			err = f.DevPodProviderUse(context.Background(), "providerX")
			framework.ExpectError(err)

			// Cleanup: delete provider 1
			err = f.DevPodProviderDelete(ctx, "provider1")
			framework.ExpectNoError(err)

			// Cleanup: ensure provider 1 is deleted
			err = f.DevPodProviderUse(context.Background(), "provider1")
			framework.ExpectError(err)
		})

		ginkgo.It("should add simple provider and update it", func() {
			tempDir, err := framework.CopyToTempDir("tests/provider/testdata/simple-k8s-provider")
			framework.ExpectNoError(err)
			defer framework.CleanupTempDir(initialDir, tempDir)

			f := framework.NewDefaultFramework(initialDir + "/bin")

			// Ensure that provider 2 is deleted
			err = f.DevPodProviderDelete(ctx, "provider2", "--ignore-not-found")
			framework.ExpectNoError(err)

			// Add provider 2 and use it
			err = f.DevPodProviderAdd(ctx, tempDir+"/provider2.yaml")
			framework.ExpectNoError(err)
			err = f.DevPodProviderUse(context.Background(), "provider2")
			framework.ExpectNoError(err)

			// Ensure provider 2 namespace parameter has the default value
			ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(30*time.Second))
			err = f.DevPodProviderOptionsCheckNamespaceDescription(ctx, "provider2", "The namespace to use")
			framework.ExpectNoError(err)
			cancel()

			// Update provider 2 (change the namespace description value)
			err = f.DevPodProviderUpdate(context.Background(), "provider2", tempDir+"/provider2-update.yaml")
			framework.ExpectNoError(err)

			// Ensure that provider 2 was updated
			ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(30*time.Second))
			err = f.DevPodProviderOptionsCheckNamespaceDescription(ctx, "provider2", "Updated namespace parameter")
			framework.ExpectNoError(err)
			cancel()

			// Cleanup: delete provider 2
			err = f.DevPodProviderDelete(context.Background(), "provider2")
			framework.ExpectNoError(err)

			// Cleanup: ensure provider 2 is deleted
			err = f.DevPodProviderUse(context.Background(), "provider2")
			framework.ExpectError(err)
		})

		ginkgo.It("should list all providers", func() {
			tempDir, err := framework.CopyToTempDir("tests/provider/testdata/simple-k8s-provider")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			f := framework.NewDefaultFramework(initialDir + "/bin")

			// Ensure that provider 1 is deleted
			err = f.DevPodProviderDelete(ctx, "provider1", "--ignore-not-found")
			framework.ExpectNoError(err)

			// Add provider 1
			err = f.DevPodProviderAdd(ctx, tempDir+"/provider1.yaml")
			framework.ExpectNoError(err)
			// Ensure provider 1 exists
			err = f.DevPodProviderUse(context.Background(), "provider1")
			framework.ExpectNoError(err)

			// Add .DS_Store file to tempDir
			err = os.Mkdir(tempDir+"/.DS_Store", 0755)
			framework.ExpectNoError(err)

			// List providers
			err = f.DevPodProviderList(context.Background())
			framework.ExpectNoError(err)

			// Cleanup: delete provider 1
			err = f.DevPodProviderDelete(ctx, "provider1")
			framework.ExpectNoError(err)

			// Cleanup: ensure provider 1 is deleted
			err = f.DevPodProviderUse(context.Background(), "provider1")
			framework.ExpectError(err)
		})

		ginkgo.It("should parse options", func() {
			tempDir, err := framework.CopyToTempDir("tests/provider/testdata/simple-k8s-provider")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			f := framework.NewDefaultFramework(initialDir + "/bin")

			// Ensure that provider is deleted
			err = f.DevPodProviderDelete(ctx, "provider3", "--ignore-not-found")
			framework.ExpectNoError(err)

			podManifest := `
apiVersion: v1
kind: Pod
metadata:
	name: test
spec:
	containers:
	- name: devpod
`
			// Add provider
			err = f.DevPodProviderAdd(ctx, tempDir+"/provider3.yaml", "--option=TEMPLATE="+podManifest)
			framework.ExpectNoError(err)
			// Ensure provider exists
			err = f.DevPodProviderUse(context.Background(), "provider3")
			framework.ExpectNoError(err)

			// look for template option
			err = f.DevPodProviderFindOption(context.Background(), "provider3", podManifest)
			framework.ExpectNoError(err)

			// Cleanup: delete provider
			err = f.DevPodProviderDelete(ctx, "provider3")
			framework.ExpectNoError(err)

			// Cleanup: ensure provider is deleted
			err = f.DevPodProviderUse(context.Background(), "provider3")
			framework.ExpectError(err)
		})

		ginkgo.Context("renaming providers", ginkgo.Label("rename"), ginkgo.Ordered, func() {
			ctx := context.Background()
			initialDir, err := os.Getwd()
			if err != nil {
				panic(err)
			}

			// RENAME-1.
			ginkgo.It("should rename a provider to a new, valid name", func() {
				tempDir, err := framework.CopyToTempDir("tests/provider/testdata/simple-k8s-provider")
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

				f := framework.NewDefaultFramework(initialDir + "/bin")

				providerName := "provider-rename1"
				renamedProviderName := "provider-renamed"

				// Ensure that provider is deleted.
				err = f.DevPodProviderDelete(ctx, providerName, "--ignore-not-found")
				framework.ExpectNoError(err)
				err = f.DevPodProviderDelete(ctx, renamedProviderName, "--ignore-not-found")
				framework.ExpectNoError(err)

				// Add provider.
				err = f.DevPodProviderAdd(ctx, tempDir+"/provider1.yaml", "--name", providerName)
				framework.ExpectNoError(err)

				// Ensure provider exists.
				err = f.DevPodProviderUse(context.Background(), providerName)
				framework.ExpectNoError(err)

				// Rename provider.
				err = f.DevPodProviderRename(context.Background(), providerName, renamedProviderName)
				framework.ExpectNoError(err)

				// Ensure old provider is gone and new one exists.
				err = f.DevPodProviderUse(context.Background(), providerName)
				framework.ExpectError(err)
				err = f.DevPodProviderUse(context.Background(), renamedProviderName)
				framework.ExpectNoError(err)

				// Cleanup: delete provider.
				err = f.DevPodProviderDelete(ctx, renamedProviderName)
				framework.ExpectNoError(err)
			})

			// RENAME-2.
			ginkgo.It("should fail to rename a provider to a name that already exists", func() {
				tempDir, err := framework.CopyToTempDir("tests/provider/testdata/simple-k8s-provider")
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

				f := framework.NewDefaultFramework(initialDir + "/bin")

				providerToRename := "provider-to-rename2"
				existingProvider := "existing-provider2"

				// Ensure that providers are deleted.
				err = f.DevPodProviderDelete(ctx, providerToRename, "--ignore-not-found")
				framework.ExpectNoError(err)
				err = f.DevPodProviderDelete(ctx, existingProvider, "--ignore-not-found")
				framework.ExpectNoError(err)

				// Add providers.
				err = f.DevPodProviderAdd(ctx, tempDir+"/provider1.yaml", "--name", providerToRename)
				framework.ExpectNoError(err)
				err = f.DevPodProviderAdd(ctx, tempDir+"/provider2.yaml", "--name", existingProvider)
				framework.ExpectNoError(err)

				// Attempt to rename provider to an existing name.
				err = f.DevPodProviderRename(context.Background(), providerToRename, existingProvider)
				framework.ExpectError(err)

				// Ensure providers still exist.
				err = f.DevPodProviderUse(context.Background(), providerToRename)
				framework.ExpectNoError(err)
				err = f.DevPodProviderUse(context.Background(), existingProvider)
				framework.ExpectNoError(err)

				// Cleanup: delete providers.
				err = f.DevPodProviderDelete(ctx, providerToRename)
				framework.ExpectNoError(err)
				err = f.DevPodProviderDelete(ctx, existingProvider)
				framework.ExpectNoError(err)
			})

			// RENAME-3.
			ginkgo.It("should fail to rename a non-existent provider", func() {
				f := framework.NewDefaultFramework(initialDir + "/bin")

				nonExistentProvider := "non-existent-provider3"
				newName := "new-name3"

				// Ensure that provider is deleted.
				err = f.DevPodProviderDelete(ctx, nonExistentProvider, "--ignore-not-found")
				framework.ExpectNoError(err)

				// Attempt to rename non-existent provider.
				err = f.DevPodProviderRename(context.Background(), nonExistentProvider, newName)
				framework.ExpectError(err)
			})

			// RENAME-4.
			ginkgo.It("should rename a provider with an associated stopped workspace", func() {
				f := framework.NewDefaultFramework(initialDir + "/bin")

				providerName := "provider-with-workspace4"
				renamedProviderName := "renamed-provider-with-workspace4"

				workspaceList, err := f.DevPodListParsed(ctx)
				framework.ExpectNoError(err)
				for _, ws := range workspaceList {
					if ws.Provider.Name == providerName {
						err = f.DevPodStop(ctx, ws.ID)
						framework.ExpectNoError(err)
						err = f.DevPodWorkspaceDelete(ctx, ws.ID)
						framework.ExpectNoError(err)
					}
				}

				// Ensure that providers are deleted.
				err = f.DevPodProviderDelete(ctx, providerName, "--ignore-not-found")
				framework.ExpectNoError(err)
				err = f.DevPodProviderDelete(ctx, renamedProviderName, "--ignore-not-found")
				framework.ExpectNoError(err)

				tempDir, err := framework.CopyToTempDir("tests/up/testdata/no-devcontainer")
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

				// Add and use provider.
				// Check if we're in a Windows CI/CD environment with Podman
				dockerHost := os.Getenv("DOCKER_HOST")
				if dockerHost != "" && strings.Contains(dockerHost, "podman") {
					err = f.DevPodProviderAdd(ctx, "docker", "--name", providerName, "--option=DOCKER_PATH=podman")
				} else {
					err = f.DevPodProviderAdd(ctx, "docker", "--name", providerName)
				}
				framework.ExpectNoError(err)
				err = f.DevPodProviderUse(ctx, providerName)
				framework.ExpectNoError(err)

				// Create and start workspace.
				err = f.DevPodUp(ctx, tempDir)
				framework.ExpectNoError(err)

				// Stop the workspace before renaming (running workspaces cannot be switched).
				err = f.DevPodStop(ctx, tempDir)
				framework.ExpectNoError(err)

				// Rename provider.
				err = f.DevPodProviderRename(ctx, providerName, renamedProviderName)
				framework.ExpectNoError(err)

				// Verify that the old provider is gone and the new one exists.
				err = f.DevPodProviderUse(ctx, providerName)
				framework.ExpectError(err)
				err = f.DevPodProviderUse(ctx, renamedProviderName)
				framework.ExpectNoError(err)

				// Start the workspace with the new provider and verify it's accessible.
				err = f.DevPodUp(ctx, tempDir)
				framework.ExpectNoError(err)

				_, err = f.DevPodSSH(ctx, tempDir, "echo 'hello'")
				framework.ExpectNoError(err)

				// Cleanup.
				err = f.DevPodStop(ctx, tempDir)
				framework.ExpectNoError(err)
				err = f.DevPodWorkspaceDelete(ctx, tempDir)
				framework.ExpectNoError(err)

				workspaceID := workspace.ToID(tempDir)
				gomega.Eventually(func() error {
					_, err := f.FindWorkspace(ctx, workspaceID)
					return err
				}).WithTimeout(30 * time.Second).
					WithPolling(1 * time.Second).
					Should(gomega.HaveOccurred())

				err = f.DevPodProviderDelete(ctx, renamedProviderName)
				framework.ExpectNoError(err)
			})

			// RENAME-5.
			ginkgo.It("should fail to rename a provider with a running workspace", func() {
				tempDir, err := framework.CopyToTempDir("tests/up/testdata/no-devcontainer")
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

				f := framework.NewDefaultFramework(initialDir + "/bin")

				providerName := "provider-with-running-workspace5"
				renamedProviderName := "renamed-provider-workspace5"

				workspaceList, err := f.DevPodListParsed(ctx)
				framework.ExpectNoError(err)
				for _, ws := range workspaceList {
					if ws.Provider.Name == providerName || ws.Provider.Name == renamedProviderName {
						err = f.DevPodStop(ctx, ws.ID)
						framework.ExpectNoError(err)
						err = f.DevPodWorkspaceDelete(ctx, ws.ID)
						framework.ExpectNoError(err)
					}
				}

				// Ensure that providers are deleted.
				err = f.DevPodProviderDelete(ctx, providerName, "--ignore-not-found")
				framework.ExpectNoError(err)
				err = f.DevPodProviderDelete(ctx, renamedProviderName, "--ignore-not-found")
				framework.ExpectNoError(err)

				// Add and use provider.
				// Check if we're in a Windows CI/CD environment with Podman.
				dockerHost := os.Getenv("DOCKER_HOST")
				if dockerHost != "" && strings.Contains(dockerHost, "podman") {
					err = f.DevPodProviderAdd(ctx, "docker", "--name", providerName, "--option=DOCKER_PATH=podman")
				} else {
					err = f.DevPodProviderAdd(ctx, "docker", "--name", providerName)
				}
				framework.ExpectNoError(err)
				err = f.DevPodProviderUse(ctx, providerName)
				framework.ExpectNoError(err)

				// Create and start workspace (workspace remains running).
				err = f.DevPodUp(ctx, tempDir)
				framework.ExpectNoError(err)

				// Attempt to rename provider - this should fail because workspace is running.
				err = f.DevPodProviderRename(ctx, providerName, renamedProviderName)
				framework.ExpectError(err)

				// Stop workspace.
				err = f.DevPodStop(ctx, tempDir)
				framework.ExpectNoError(err)

				// Verify that the old provider still exists and the new one doesn't.
				err = f.DevPodProviderUse(ctx, providerName)
				framework.ExpectNoError(err)
				err = f.DevPodProviderUse(ctx, renamedProviderName)
				framework.ExpectError(err)

				// Cleanup.
				err = f.DevPodWorkspaceDelete(ctx, tempDir)
				framework.ExpectNoError(err)
				err = f.DevPodProviderDelete(ctx, providerName)
				framework.ExpectNoError(err)
			})

			// RENAME-6.
			ginkgo.It("should fail to rename a provider to an invalid name", func() {
				tempDir, err := framework.CopyToTempDir("tests/provider/testdata/simple-k8s-provider")
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

				f := framework.NewDefaultFramework(initialDir + "/bin")

				providerName := "provider-to-rename-invalid6"

				// Ensure that provider is deleted.
				err = f.DevPodProviderDelete(ctx, providerName, "--ignore-not-found")
				framework.ExpectNoError(err)

				// Add provider.
				err = f.DevPodProviderAdd(ctx, tempDir+"/provider1.yaml", "--name", providerName)
				framework.ExpectNoError(err)

				// Attempt to rename provider to an invalid name.
				err = f.DevPodProviderRename(context.Background(), providerName, "invalid/name6")
				framework.ExpectError(err)

				// Ensure provider still exists.
				err = f.DevPodProviderUse(context.Background(), providerName)
				framework.ExpectNoError(err)

				// Cleanup: delete provider.
				err = f.DevPodProviderDelete(ctx, providerName)
				framework.ExpectNoError(err)
			})
		})
	})
})
