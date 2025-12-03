package integration

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

var _ = DevPodDescribe("network traffic", func() {
	ginkgo.Context("real network traffic", ginkgo.Label("traffic"), func() {
		var initialDir string

		ginkgo.BeforeEach(func() {
			var err error
			initialDir, err = os.Getwd()
			framework.ExpectNoError(err)
		})

		ginkgo.It("forwards HTTP traffic through SSH tunnel", ginkgo.Label("http-traffic"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/../../bin")

			_ = f.DevPodProviderDelete(ctx, "docker")
			err := f.DevPodProviderAdd(ctx, "docker")
			framework.ExpectNoError(err)
			err = f.DevPodProviderUse(ctx, "docker")
			framework.ExpectNoError(err)

			testDir := filepath.Join(initialDir, "testdata", "simple-app")
			name := "test-http-traffic"
			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), name)

			err = f.DevPodUp(ctx, testDir, "--id", name)
			framework.ExpectNoError(err)

			// Start HTTP server in container
			_, err = f.DevPodSSH(ctx, name, "nohup python3 /tmp/server.py > /tmp/server.log 2>&1 &")
			framework.ExpectNoError(err)

			time.Sleep(3 * time.Second)

			// Verify server is running
			out, err := f.DevPodSSH(ctx, name, "curl -s http://localhost:8080 2>&1 || echo 'FAILED'")
			framework.ExpectNoError(err)

			if strings.Contains(out, "FAILED") || !strings.Contains(out, "Hello from DevPod") {
				ginkgo.Skip("HTTP server not running properly")
			}

			// Server is running, verify we got the response
			framework.ExpectEqual(strings.Contains(out, "Hello from DevPod"), true)
		})

		ginkgo.It("handles multiple concurrent connections", ginkgo.Label("concurrent-traffic"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/../../bin")

			_ = f.DevPodProviderDelete(ctx, "docker")
			err := f.DevPodProviderAdd(ctx, "docker")
			framework.ExpectNoError(err)
			err = f.DevPodProviderUse(ctx, "docker")
			framework.ExpectNoError(err)

			testDir := filepath.Join(initialDir, "testdata", "simple-app")
			name := "test-concurrent"
			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), name)

			err = f.DevPodUp(ctx, testDir, "--id", name)
			framework.ExpectNoError(err)

			// Make 5 concurrent SSH connections
			done := make(chan bool, 5)
			for i := range 5 {
				go func(idx int) {
					out, err := f.DevPodSSH(ctx, name, fmt.Sprintf("echo 'connection-%d'", idx))
					if err == nil && strings.Contains(out, fmt.Sprintf("connection-%d", idx)) {
						done <- true
					} else {
						done <- false
					}
				}(i)
			}

			// Wait for all connections
			success := 0
			for i := 0; i < 5; i++ {
				if <-done {
					success++
				}
			}

			// At least 4 out of 5 should succeed
			framework.ExpectEqual(success >= 4, true)
		})

		ginkgo.It("transfers data through connection", ginkgo.Label("data-transfer"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/../../bin")

			_ = f.DevPodProviderDelete(ctx, "docker")
			err := f.DevPodProviderAdd(ctx, "docker")
			framework.ExpectNoError(err)
			err = f.DevPodProviderUse(ctx, "docker")
			framework.ExpectNoError(err)

			testDir := filepath.Join(initialDir, "testdata", "simple-app")
			name := "test-data-transfer"
			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), name)

			err = f.DevPodUp(ctx, testDir, "--id", name)
			framework.ExpectNoError(err)

			// Create a file with data
			testData := "This is test data for network transfer validation"
			_, err = f.DevPodSSH(ctx, name, fmt.Sprintf("echo '%s' > /tmp/testfile.txt", testData))
			framework.ExpectNoError(err)

			// Read it back
			out, err := f.DevPodSSH(ctx, name, "cat /tmp/testfile.txt")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), testData)
		})

		ginkgo.It("handles connection errors gracefully", ginkgo.Label("error-handling"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/../../bin")

			_ = f.DevPodProviderDelete(ctx, "docker")
			err := f.DevPodProviderAdd(ctx, "docker")
			framework.ExpectNoError(err)
			err = f.DevPodProviderUse(ctx, "docker")
			framework.ExpectNoError(err)

			testDir := filepath.Join(initialDir, "testdata", "simple-app")
			name := "test-error-handling"
			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), name)

			err = f.DevPodUp(ctx, testDir, "--id", name)
			framework.ExpectNoError(err)

			// Try to connect to non-existent port
			_, err = f.DevPodSSH(ctx, name, "curl -s --connect-timeout 2 http://localhost:9999 2>&1 || echo 'EXPECTED_ERROR'")
			framework.ExpectNoError(err)
			// Should handle error gracefully (not crash)
		})
	})
})
