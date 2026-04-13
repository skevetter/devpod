package machineprovider

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = ginkgo.Describe(
	"devpod machine provider test suite",
	ginkgo.Label("machineprovider"),
	ginkgo.Ordered,
	func() {
		var initialDir string

		ginkgo.BeforeEach(func() {
			var err error
			initialDir, err = os.Getwd()
			framework.ExpectNoError(err)
		})

		ginkgo.It("test start / stop / status",
			ginkgo.SpecTimeout(framework.GetTimeout()),
			func(ctx context.Context) {
				f := framework.NewDefaultFramework(initialDir + "/bin")

				// copy test dir
				tempDir, err := framework.CopyToTempDirWithoutChdir(
					initialDir + "/tests/machineprovider/testdata/machineprovider",
				)
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

				// create docker provider
				err = f.DevPodProviderAdd(
					ctx,
					filepath.Join(tempDir, "provider.yaml"),
				)
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(func(cleanupCtx context.Context) {
					err = f.DevPodWorkspaceDelete(cleanupCtx, tempDir)
					framework.ExpectNoError(err)
				})

				// wait for devpod workspace to come online (deadline: 30s)
				err = f.DevPodUp(ctx, tempDir, "--debug")
				framework.ExpectNoError(err)

				// expect workspace
				workspace, err := f.FindWorkspace(ctx, tempDir)
				framework.ExpectNoError(err)

				// check status
				status, err := f.DevPodStatus(ctx, tempDir)
				framework.ExpectNoError(err)
				framework.ExpectEqual(
					strings.ToUpper(status.State),
					"RUNNING",
					"workspace status did not match",
				)

				// stop container
				err = f.DevPodStop(ctx, tempDir)
				framework.ExpectNoError(err)

				// check status
				status, err = f.DevPodStatus(ctx, tempDir)
				framework.ExpectNoError(err)
				framework.ExpectEqual(
					strings.ToUpper(status.State),
					"STOPPED",
					"workspace status did not match",
				)

				// wait for devpod workspace to come online (deadline: 30s)
				err = f.DevPodUp(ctx, tempDir)
				framework.ExpectNoError(err)

				// check if ssh works as it should start the container
				out, err := f.DevPodSSH(
					ctx,
					tempDir,
					fmt.Sprintf("cat /workspaces/%s/test.txt", workspace.ID),
				)
				framework.ExpectNoError(err)
				framework.ExpectEqual(
					strings.TrimSpace(out),
					"Test123",
					"workspace content does not match",
				)
			})

		ginkgo.It("test devpod inactivity timeout",
			ginkgo.SpecTimeout(framework.GetTimeout()*5),
			func(ctx context.Context) {
				f := framework.NewDefaultFramework(initialDir + "/bin")

				// copy test dir
				tempDir, err := framework.CopyToTempDirWithoutChdir(
					initialDir + "/tests/machineprovider/testdata/machineprovider2",
				)
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

				// create provider
				_ = f.DevPodProviderDelete(ctx, "docker123")
				err = f.DevPodProviderAdd(ctx, filepath.Join(tempDir, "provider.yaml"))
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(func(cleanupCtx context.Context) {
					err = f.DevPodWorkspaceDelete(cleanupCtx, tempDir)
					framework.ExpectNoError(err)
					err = f.DevPodProviderDelete(cleanupCtx, "docker123")
					framework.ExpectNoError(err)
				})

				// wait for devpod workspace to come online (deadline: 30s)
				err = f.DevPodUp(ctx, tempDir, "--debug", "--daemon-interval=3s")
				framework.ExpectNoError(err)

				// check status
				status, err := f.DevPodStatus(ctx, tempDir, "--container-status=false")
				framework.ExpectNoError(err)
				framework.ExpectEqual(
					strings.ToUpper(status.State),
					"RUNNING",
					"workspace status did not match",
				)

				// stop container
				err = f.DevPodStop(ctx, tempDir)
				framework.ExpectNoError(err)

				// check status
				status, err = f.DevPodStatus(ctx, tempDir, "--container-status=false")
				framework.ExpectNoError(err)
				framework.ExpectEqual(
					strings.ToUpper(status.State),
					"STOPPED",
					"workspace status did not match",
				)

				// wait for devpod workspace to come online (deadline: 30s)
				err = f.DevPodUp(ctx, tempDir, "--daemon-interval=3s")
				framework.ExpectNoError(err)

				// check status
				status, err = f.DevPodStatus(ctx, tempDir, "--container-status=false")
				framework.ExpectNoError(err)
				framework.ExpectEqual(
					strings.ToUpper(status.State),
					"RUNNING",
					"workspace status did not match",
				)

				// wait until workspace is stopped again
				gomega.Eventually(func() string {
					status, err := f.DevPodStatus(ctx, tempDir, "--container-status=false")
					framework.ExpectNoError(err)
					return strings.ToUpper(status.State)
				}, time.Minute*2, time.Second*2).Should(
					gomega.Equal("STOPPED"),
					"machine did not shutdown in time",
				)
			})
	},
)
