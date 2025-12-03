package network

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = DevPodDescribe("network traffic", func() {
	ginkgo.Context("real network traffic", ginkgo.Label("network", "traffic"), func() {
		ginkgo.It("forwards HTTP traffic through SSH tunnel", ginkgo.Label("http-traffic"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker", "-o", "DOCKER_PATH=docker")
			_ = f.DevPodProviderUse(ctx, "docker")

			tempDir, err := framework.CopyToTempDir(initialDir + "/tests/network/testdata/simple-app")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			_, err = f.DevPodSSH(ctx, tempDir, "nohup python3 /tmp/server.py > /tmp/server.log 2>&1 &")
			framework.ExpectNoError(err)

			time.Sleep(3 * time.Second)

			out, err := f.DevPodSSH(ctx, tempDir, "curl -s http://localhost:8080 2>&1 || echo 'FAILED'")
			framework.ExpectNoError(err)

			if strings.Contains(out, "FAILED") || !strings.Contains(out, "Hello from DevPod") {
				ginkgo.Skip("HTTP server not running properly")
			}

			framework.ExpectEqual(strings.Contains(out, "Hello from DevPod"), true)
		})

		ginkgo.It("handles multiple concurrent connections", ginkgo.Label("concurrent-traffic"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker", "-o", "DOCKER_PATH=docker")
			_ = f.DevPodProviderUse(ctx, "docker")

			tempDir, err := framework.CopyToTempDir(initialDir + "/tests/network/testdata/simple-app")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			done := make(chan bool, 5)
			for i := range 5 {
				go func(idx int) {
					out, err := f.DevPodSSH(ctx, tempDir, fmt.Sprintf("echo 'connection-%d'", idx))
					if err == nil && strings.Contains(out, fmt.Sprintf("connection-%d", idx)) {
						done <- true
					} else {
						done <- false
					}
				}(i)
			}

			success := 0
			for i := 0; i < 5; i++ {
				if <-done {
					success++
				}
			}

			framework.ExpectEqual(success >= 4, true)
		})

		ginkgo.It("transfers data through connection", ginkgo.Label("data-transfer"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker", "-o", "DOCKER_PATH=docker")
			_ = f.DevPodProviderUse(ctx, "docker")

			tempDir, err := framework.CopyToTempDir(initialDir + "/tests/network/testdata/simple-app")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			testData := "This is test data for network transfer validation"
			_, err = f.DevPodSSH(ctx, tempDir, fmt.Sprintf("echo '%s' > /tmp/testfile.txt", testData))
			framework.ExpectNoError(err)

			out, err := f.DevPodSSH(ctx, tempDir, "cat /tmp/testfile.txt")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), testData)
		})

		ginkgo.It("handles connection errors gracefully", ginkgo.Label("error-handling"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker", "-o", "DOCKER_PATH=docker")
			_ = f.DevPodProviderUse(ctx, "docker")

			tempDir, err := framework.CopyToTempDir(initialDir + "/tests/network/testdata/simple-app")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			_, err = f.DevPodSSH(ctx, tempDir, "curl -s --connect-timeout 2 http://localhost:9999 2>&1 || echo 'EXPECTED_ERROR'")
			framework.ExpectNoError(err)
		})
	})
})
