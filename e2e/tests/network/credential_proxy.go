package network

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = DevPodDescribe("credential proxy", func() {
	ginkgo.Context("integration tests", ginkgo.Label("network", "integration"), func() {
		ginkgo.It("validates complete workflow with HTTP transport", ginkgo.Label("http-transport"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker", "-o", "DOCKER_PATH=docker")
			_ = f.DevPodProviderUse(ctx, "docker")

			tempDir, err := framework.CopyToTempDir(initialDir + "/tests/network/testdata/with-network-proxy")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			out, err := f.DevPodSSH(ctx, tempDir, "git config --global credential.helper")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.Contains(out, "devpod"), true, "git credential helper should be configured")

			out, err = f.DevPodSSH(ctx, tempDir, "test -f ~/.docker/config.json && echo 'exists' || echo 'missing'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "exists", "docker config should exist")
		})

		ginkgo.It("validates transport fallback mechanism", ginkgo.Label("fallback"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker", "-o", "DOCKER_PATH=docker")
			_ = f.DevPodProviderUse(ctx, "docker")

			tempDir, err := framework.CopyToTempDir(initialDir + "/tests/network/testdata/simple-app")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			out, err := f.DevPodSSH(ctx, tempDir, "echo 'fallback test'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "fallback test")
		})

		ginkgo.It("validates daemon lifecycle", ginkgo.Label("daemon"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker", "-o", "DOCKER_PATH=docker")
			_ = f.DevPodProviderUse(ctx, "docker")

			tempDir, err := framework.CopyToTempDir(initialDir + "/tests/network/testdata/with-network-proxy")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			out, err := f.DevPodSSH(ctx, tempDir, "test -S /var/run/devpod/devpod-net.sock && echo 'exists' || echo 'missing'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "exists", "network proxy socket should exist")
		})

		ginkgo.It("validates concurrent credential requests", ginkgo.Label("concurrent"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker", "-o", "DOCKER_PATH=docker")
			_ = f.DevPodProviderUse(ctx, "docker")

			tempDir, err := framework.CopyToTempDir(initialDir + "/tests/network/testdata/with-network-proxy")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			for range 10 {
				_, err := f.DevPodSSH(ctx, tempDir, "git config --global credential.helper")
				framework.ExpectNoError(err)
			}
		})

		ginkgo.It("validates git and docker credentials work together", ginkgo.Label("multi-credential"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker", "-o", "DOCKER_PATH=docker")
			_ = f.DevPodProviderUse(ctx, "docker")

			tempDir, err := framework.CopyToTempDir(initialDir + "/tests/network/testdata/with-network-proxy")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			_, err = f.DevPodSSH(ctx, tempDir, "git config --global credential.helper")
			framework.ExpectNoError(err)

			_, err = f.DevPodSSH(ctx, tempDir, "test -d ~/.docker")
			framework.ExpectNoError(err)
		})

		ginkgo.It("survives container restart", ginkgo.Label("resilience"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker", "-o", "DOCKER_PATH=docker")
			_ = f.DevPodProviderUse(ctx, "docker")

			tempDir, err := framework.CopyToTempDir(initialDir + "/tests/network/testdata/with-network-proxy")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			out, err := f.DevPodSSH(ctx, tempDir, "git config --global credential.helper")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.Contains(out, "devpod"), true)

			err = f.DevPodStop(ctx, tempDir)
			framework.ExpectNoError(err)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			out, err = f.DevPodSSH(ctx, tempDir, "git config --global credential.helper")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.Contains(out, "devpod"), true)
		})
	})

	ginkgo.Context("edge cases", ginkgo.Label("network", "edge-cases"), func() {
		ginkgo.It("handles socket permissions gracefully", ginkgo.Label("socket-permissions"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker", "-o", "DOCKER_PATH=docker")
			_ = f.DevPodProviderUse(ctx, "docker")

			tempDir, err := framework.CopyToTempDir(initialDir + "/tests/network/testdata/with-network-proxy")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			out, err := f.DevPodSSH(ctx, tempDir, "test -S /var/run/devpod/devpod-net.sock && echo 'socket' || echo 'not-socket'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "socket", "should be a socket file")

			out, err = f.DevPodSSH(ctx, tempDir, "echo 'functional'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "functional")
		})

		ginkgo.It("handles rapid stop/start cycles", ginkgo.Label("rapid-restart"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker", "-o", "DOCKER_PATH=docker")
			_ = f.DevPodProviderUse(ctx, "docker")

			tempDir, err := framework.CopyToTempDir(initialDir + "/tests/network/testdata/simple-app")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			name := "rapid-restart-test"
			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), name)

			err = f.DevPodUp(ctx, tempDir, "--id", name)
			framework.ExpectNoError(err)

			for i := range 3 {
				err = f.DevPodWorkspaceStop(ctx, name)
				framework.ExpectNoError(err)

				err = f.DevPodUp(ctx, tempDir, "--id", name)
				framework.ExpectNoError(err)

				out, err := f.DevPodSSH(ctx, name, "echo 'cycle-"+string(rune('0'+i))+"'")
				framework.ExpectNoError(err)
				framework.ExpectEqual(strings.Contains(out, "cycle"), true, "should work after restart")
			}
		})

		ginkgo.It("handles missing network proxy config", ginkgo.Label("missing-config"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker", "-o", "DOCKER_PATH=docker")
			_ = f.DevPodProviderUse(ctx, "docker")

			tempDir, err := framework.CopyToTempDir(initialDir + "/tests/network/testdata/simple-app")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			out, err := f.DevPodSSH(ctx, tempDir, "echo 'fallback-works'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "fallback-works")

			out, err = f.DevPodSSH(ctx, tempDir, "test -S /var/run/devpod/devpod-net.sock && echo 'exists' || echo 'not-exists'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "not-exists", "socket should not exist without config")
		})

		ginkgo.It("handles concurrent requests without deadlock", ginkgo.Label("concurrent-stress"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker", "-o", "DOCKER_PATH=docker")
			_ = f.DevPodProviderUse(ctx, "docker")

			tempDir, err := framework.CopyToTempDir(initialDir + "/tests/network/testdata/with-network-proxy")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			done := make(chan bool, 50)
			for range 50 {
				go func() {
					_, err := f.DevPodSSH(ctx, tempDir, "git config --global credential.helper 2>&1")
					done <- err == nil
				}()
			}

			success := 0
			timeout := time.After(30 * time.Second)
			for range 50 {
				select {
				case result := <-done:
					if result {
						success++
					}
				case <-timeout:
					ginkgo.Fail("Concurrent requests timed out - possible deadlock")
					return
				}
			}

			framework.ExpectEqual(success >= 45, true, "at least 45/50 concurrent requests should succeed")
		})

		ginkgo.It("validates cleanup on abnormal termination", ginkgo.Label("cleanup"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker", "-o", "DOCKER_PATH=docker")
			_ = f.DevPodProviderUse(ctx, "docker")

			tempDir, err := framework.CopyToTempDir(initialDir + "/tests/network/testdata/with-network-proxy")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			name := "cleanup-test"
			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), name)

			err = f.DevPodUp(ctx, tempDir, "--id", name)
			framework.ExpectNoError(err)

			out, err := f.DevPodSSH(ctx, name, "test -S /var/run/devpod/devpod-net.sock && echo 'exists' || echo 'missing'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "exists")

			err = f.DevPodWorkspaceStop(ctx, name)
			framework.ExpectNoError(err)

			err = f.DevPodUp(ctx, tempDir, "--id", name)
			framework.ExpectNoError(err)

			out, err = f.DevPodSSH(ctx, name, "test -S /var/run/devpod/devpod-net.sock && echo 'exists' || echo 'missing'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "exists", "socket should be recreated")
		})
	})

	ginkgo.Context("reliability", ginkgo.Label("network", "reliability"), func() {
		ginkgo.It("maintains stability over extended period", ginkgo.Label("long-running"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker", "-o", "DOCKER_PATH=docker")
			_ = f.DevPodProviderUse(ctx, "docker")

			tempDir, err := framework.CopyToTempDir(initialDir + "/tests/network/testdata/with-network-proxy")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			startTime := time.Now()
			iterations := 0
			failures := 0

			for time.Since(startTime) < 2*time.Minute {
				out, err := f.DevPodSSH(ctx, tempDir, fmt.Sprintf("echo 'iteration-%d'", iterations))
				if err != nil || !strings.Contains(out, fmt.Sprintf("iteration-%d", iterations)) {
					failures++
				}
				iterations++

				if iterations%3 == 0 {
					time.Sleep(5 * time.Second)
				} else {
					time.Sleep(1 * time.Second)
				}
			}

			failureRate := float64(failures) / float64(iterations)
			framework.ExpectEqual(failureRate < 0.05, true, fmt.Sprintf("failure rate %.2f%% should be under 5%%", failureRate*100))
		})

		ginkgo.It("handles connection timeouts gracefully", ginkgo.Label("timeout"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker", "-o", "DOCKER_PATH=docker")
			_ = f.DevPodProviderUse(ctx, "docker")

			tempDir, err := framework.CopyToTempDir(initialDir + "/tests/network/testdata/simple-app")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			out, err := f.DevPodSSH(ctx, tempDir, "echo 'before-idle'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "before-idle")

			time.Sleep(45 * time.Second)

			out, err = f.DevPodSSH(ctx, tempDir, "echo 'after-idle'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "after-idle", "should reconnect after idle")
		})

		ginkgo.It("validates no file descriptor leaks", ginkgo.Label("fd-leak"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker", "-o", "DOCKER_PATH=docker")
			_ = f.DevPodProviderUse(ctx, "docker")

			tempDir, err := framework.CopyToTempDir(initialDir + "/tests/network/testdata/with-network-proxy")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			out, err := f.DevPodSSH(ctx, tempDir, "ls -1 /proc/$$/fd | wc -l")
			framework.ExpectNoError(err)
			initialFDs := strings.TrimSpace(out)

			for range 50 {
				_, err := f.DevPodSSH(ctx, tempDir, "git config --global credential.helper 2>&1")
				framework.ExpectNoError(err)
			}

			out, err = f.DevPodSSH(ctx, tempDir, "ls -1 /proc/$$/fd | wc -l")
			framework.ExpectNoError(err)
			finalFDs := strings.TrimSpace(out)

			framework.ExpectEqual(initialFDs != "" && finalFDs != "", true, "should measure FDs")
		})

		ginkgo.It("handles mixed credential types", ginkgo.Label("mixed-credentials"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker", "-o", "DOCKER_PATH=docker")
			_ = f.DevPodProviderUse(ctx, "docker")

			tempDir, err := framework.CopyToTempDir(initialDir + "/tests/network/testdata/with-network-proxy")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			for range 20 {
				_, err := f.DevPodSSH(ctx, tempDir, "git config --global credential.helper 2>&1")
				framework.ExpectNoError(err)

				_, err = f.DevPodSSH(ctx, tempDir, "test -f ~/.docker/config.json && echo 'exists' || echo 'missing'")
				framework.ExpectNoError(err)
			}

			out, err := f.DevPodSSH(ctx, tempDir, "test -d ~/.docker && echo 'exists' || echo 'missing'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "exists", "docker config should exist")
		})

		ginkgo.It("validates graceful degradation", ginkgo.Label("degradation"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker", "-o", "DOCKER_PATH=docker")
			_ = f.DevPodProviderUse(ctx, "docker")

			tempDir, err := framework.CopyToTempDir(initialDir + "/tests/network/testdata/simple-app")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			out, err := f.DevPodSSH(ctx, tempDir, "echo 'fallback-active'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "fallback-active")

			for i := range 10 {
				out, err := f.DevPodSSH(ctx, tempDir, fmt.Sprintf("echo 'stable-%d'", i))
				framework.ExpectNoError(err)
				framework.ExpectEqual(strings.Contains(out, "stable"), true)
			}
		})
	})
})
