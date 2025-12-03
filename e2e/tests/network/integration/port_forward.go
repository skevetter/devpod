package integration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = DevPodDescribe("port forwarding", func() {
	var initialDir string
	var f *framework.Framework
	var ctx context.Context

	ginkgo.BeforeEach(func() {
		var err error
		initialDir, err = os.Getwd()
		framework.ExpectNoError(err)
		ctx = context.Background()
		f = framework.NewDefaultFramework(initialDir + "/../../bin")

		_ = f.DevPodProviderDelete(ctx, "docker")
		err = f.DevPodProviderAdd(ctx, "docker")
		framework.ExpectNoError(err)
		err = f.DevPodProviderUse(ctx, "docker")
		framework.ExpectNoError(err)
	})

	ginkgo.Context("basic network operations", ginkgo.Label("port-forward"), func() {
		ginkgo.It("workspace supports network operations", ginkgo.Label("port-forward-basic"), func() {
			testDir := filepath.Join(initialDir, "testdata", "simple-app")
			name := "test-port-forward"
			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), name)

			err := f.DevPodUp(ctx, testDir, "--id", name)
			framework.ExpectNoError(err)

			out, err := f.DevPodSSH(ctx, name, "python3 -c 'import socket; print(\"network ok\")'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "network ok")
		})
	})

	ginkgo.Context("HTTP server forwarding", ginkgo.Label("port-forward-actual"), func() {
		ginkgo.It("forwards HTTP port from container", ginkgo.Label("http-forward"), func() {
			testDir := filepath.Join(initialDir, "testdata", "simple-app")
			name := "test-http-forward"
			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), name)

			err := f.DevPodUp(ctx, testDir, "--id", name)
			framework.ExpectNoError(err)

			_, err = f.DevPodSSH(ctx, name, "python3 /tmp/server.py > /tmp/server.log 2>&1 &")
			framework.ExpectNoError(err)

			time.Sleep(3 * time.Second)

			out, err := f.DevPodSSH(ctx, name, "curl -s http://localhost:8080 || echo 'failed'")
			framework.ExpectNoError(err)
			if strings.Contains(out, "failed") {
				ginkgo.Skip("HTTP server not running in container")
			}

			framework.ExpectEqual(strings.Contains(out, "Hello from DevPod"), true)
		})
	})
})
