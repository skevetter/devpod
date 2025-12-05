package integration

import (
	"context"
	"os"
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
		f = setupDockerProvider(initialDir + "/bin")
	})

	ginkgo.Context("basic network operations", ginkgo.Label("port-forward"), func() {
		ginkgo.It("workspace supports network operations", ginkgo.Label("port-forward-basic"), func() {
			tempDir, err := framework.CopyToTempDir("tests/network/testdata/simple-app")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			out, err := f.DevPodSSH(ctx, tempDir, "python3 -c 'import socket; print(\"network ok\")'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "network ok")
		})
	})

	ginkgo.Context("HTTP server forwarding", ginkgo.Label("port-forward-actual"), func() {
		ginkgo.It("forwards HTTP port from container", ginkgo.Label("http-forward"), func() {
			tempDir, err := framework.CopyToTempDir("tests/network/testdata/simple-app")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			_, err = f.DevPodSSH(ctx, tempDir, "nohup python3 /tmp/server.py > /tmp/server.log 2>&1 &")
			framework.ExpectNoError(err)

			time.Sleep(3 * time.Second)

			out, err := f.DevPodSSH(ctx, tempDir, "curl -s http://localhost:8080 || echo 'failed'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.Contains(out, "Hello from DevPod"), true)
		})
	})
})
