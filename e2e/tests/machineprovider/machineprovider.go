package machineprovider

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
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

		ginkgo.It("test start / stop / status", func(ctx context.Context) {
			f := framework.NewDefaultFramework(initialDir + "/bin")

			// copy test dir
			tempDir, err := framework.CopyToTempDirWithoutChdir(
				initialDir + "/tests/machineprovider/testdata/machineprovider",
			)
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(func() {
				_ = os.RemoveAll(tempDir)
			})

			// create docker provider
			err = f.DevPodProviderAdd(
				ctx,
				filepath.Join(tempDir, "provider.yaml"),
			)
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(func(cleanupCtx context.Context) {
				err = f.DevPodProviderDelete(cleanupCtx, "docker123")
				framework.ExpectNoError(err)

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
		}, ginkgo.SpecTimeout(framework.GetTimeout()))

		ginkgo.It("test devpod inactivity timeout", func(ctx context.Context) {
			f := framework.NewDefaultFramework(initialDir + "/bin")

			// copy test dir
			tempDir, err := framework.CopyToTempDirWithoutChdir(
				initialDir + "/tests/machineprovider/testdata/machineprovider2",
			)
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(func() {
				err = os.RemoveAll(tempDir)
				framework.ExpectNoError(err)
			})

			// create provider
			_ = f.DevPodProviderDelete(ctx, "docker123")
			err = f.DevPodProviderAdd(ctx, filepath.Join(tempDir, "provider.yaml"))
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(func(cleanupCtx context.Context) {
				err = f.DevPodProviderDelete(cleanupCtx, "docker123")
				framework.ExpectNoError(err)

				err = f.DevPodWorkspaceDelete(cleanupCtx, tempDir)
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
			now := time.Now()
			for {
				status, err := f.DevPodStatus(ctx, tempDir, "--container-status=false")
				framework.ExpectNoError(err)
				framework.ExpectEqual(
					time.Since(now) < time.Minute*2,
					true,
					"machine did not shutdown in time",
				)
				if strings.EqualFold(status.State, "STOPPED") {
					break
				}

				time.Sleep(time.Second * 2)
			}
		}, ginkgo.SpecTimeout(framework.GetTimeout()*5))
	},
)
