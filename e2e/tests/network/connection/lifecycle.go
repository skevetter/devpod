package connection

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = DevPodDescribe("connection lifecycle", func() {
	ginkgo.Context("connection management", ginkgo.Label("lifecycle"), func() {
		var initialDir string

		ginkgo.BeforeEach(func() {
			var err error
			initialDir, err = os.Getwd()
			framework.ExpectNoError(err)
		})

		ginkgo.It("handles connection open and close", ginkgo.Label("open-close"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/../../bin")

			_ = f.DevPodProviderDelete(ctx, "docker")
			err := f.DevPodProviderAdd(ctx, "docker")
			framework.ExpectNoError(err)
			err = f.DevPodProviderUse(ctx, "docker")
			framework.ExpectNoError(err)

			testDir := filepath.Join(initialDir, "testdata", "with-network-proxy")
			name := "test-open-close"
			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), name)

			err = f.DevPodUp(ctx, testDir, "--id", name)
			framework.ExpectNoError(err)

			// Open connection
			out, err := f.DevPodSSH(ctx, name, "echo 'opened'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "opened")

			// Connection should close after command
			// Open new connection
			out, err = f.DevPodSSH(ctx, name, "echo 'reopened'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "reopened")
		})

		ginkgo.It("handles rapid connection cycling", ginkgo.Label("rapid-cycling"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/../../bin")

			_ = f.DevPodProviderDelete(ctx, "docker")
			err := f.DevPodProviderAdd(ctx, "docker")
			framework.ExpectNoError(err)
			err = f.DevPodProviderUse(ctx, "docker")
			framework.ExpectNoError(err)

			testDir := filepath.Join(initialDir, "testdata", "simple-app")
			name := "test-rapid-cycling"
			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), name)

			err = f.DevPodUp(ctx, testDir, "--id", name)
			framework.ExpectNoError(err)

			// Rapidly open and close connections
			for i := 0; i < 20; i++ {
				_, err := f.DevPodSSH(ctx, name, "echo 'ping'")
				framework.ExpectNoError(err)
			}
		})

		ginkgo.It("maintains connection after idle period", ginkgo.Label("idle-connection"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/../../bin")

			_ = f.DevPodProviderDelete(ctx, "docker")
			err := f.DevPodProviderAdd(ctx, "docker")
			framework.ExpectNoError(err)
			err = f.DevPodProviderUse(ctx, "docker")
			framework.ExpectNoError(err)

			testDir := filepath.Join(initialDir, "testdata", "simple-app")
			name := "test-idle"
			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), name)

			err = f.DevPodUp(ctx, testDir, "--id", name)
			framework.ExpectNoError(err)

			// First connection
			out, err := f.DevPodSSH(ctx, name, "echo 'before idle'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "before idle")

			// Wait 5 seconds (idle period)
			time.Sleep(5 * time.Second)

			// Connection after idle
			out, err = f.DevPodSSH(ctx, name, "echo 'after idle'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "after idle")
		})

		ginkgo.It("recovers from command failures", ginkgo.Label("error-recovery"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/../../bin")

			_ = f.DevPodProviderDelete(ctx, "docker")
			err := f.DevPodProviderAdd(ctx, "docker")
			framework.ExpectNoError(err)
			err = f.DevPodProviderUse(ctx, "docker")
			framework.ExpectNoError(err)

			testDir := filepath.Join(initialDir, "testdata", "simple-app")
			name := "test-recovery"
			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), name)

			err = f.DevPodUp(ctx, testDir, "--id", name)
			framework.ExpectNoError(err)

			// Run failing command
			_, _ = f.DevPodSSH(ctx, name, "exit 1")

			// Should still be able to connect
			out, err := f.DevPodSSH(ctx, name, "echo 'recovered'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "recovered")
		})
	})
})
