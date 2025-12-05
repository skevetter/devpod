package network

import (
	"context"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = DevPodDescribe("port forwarding", func() {
	ginkgo.Context("basic network operations", ginkgo.Label("network", "port-forward"), func() {
		ginkgo.It("workspace supports network operations", ginkgo.Label("port-forward-basic"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker", "-o", "DOCKER_PATH=docker")
			_ = f.DevPodProviderUse(ctx, "docker")

			tempDir, err := framework.CopyToTempDir(initialDir + "/tests/network/testdata/simple-app")
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
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker", "-o", "DOCKER_PATH=docker")
			_ = f.DevPodProviderUse(ctx, "docker")

			tempDir, err := framework.CopyToTempDir(initialDir + "/tests/network/testdata/simple-app")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			_, err = f.DevPodSSH(ctx, tempDir, "/tmp/start_server.sh")
			framework.ExpectNoError(err)

			time.Sleep(3 * time.Second)

			out, err := f.DevPodSSH(ctx, tempDir, "python3 -c 'import urllib.request; print(urllib.request.urlopen(\"http://localhost:8080\").read().decode())' 2>/dev/null || echo 'failed'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.Contains(out, "Hello from DevPod"), true)
		})
	})
})
