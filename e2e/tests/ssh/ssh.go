package ssh

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = DevPodDescribe("devpod ssh test suite", func() {
	ginkgo.Context("testing ssh command", ginkgo.Label("ssh"), ginkgo.Ordered, func() {
		ctx := context.Background()
		initialDir, err := os.Getwd()
		if err != nil {
			panic(err)
		}

		ginkgo.It("should start a new workspace with a docker provider (default) and ssh into it", func() {
			tempDir, err := framework.CopyToTempDir("tests/ssh/testdata/local-test")
			framework.ExpectNoError(err)
			defer framework.CleanupTempDir(initialDir, tempDir)

			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker")
			err = f.DevPodProviderUse(context.Background(), "docker")
			framework.ExpectNoError(err)

			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), tempDir)

			// Start up devpod workspace
			devpodUpDeadline := time.Now().Add(5 * time.Minute)
			devpodUpCtx, cancel := context.WithDeadline(context.Background(), devpodUpDeadline)
			defer cancel()
			err = f.DevPodUp(devpodUpCtx, tempDir)
			framework.ExpectNoError(err)

			devpodSSHDeadline := time.Now().Add(20 * time.Second)
			devpodSSHCtx, cancelSSH := context.WithDeadline(context.Background(), devpodSSHDeadline)
			defer cancelSSH()
			err = f.DevPodSSHEchoTestString(devpodSSHCtx, tempDir)
			framework.ExpectNoError(err)
		})

		// ginkgo.It("should start a new workspace with a docker provider (default) and forward gpg agent into it", func() {
		// 	// skip windows for now
		// 	if runtime.GOOS == "windows" {
		// 		return
		// 	}
		//
		// 	tempDir, err := framework.CopyToTempDir("tests/ssh/testdata/gpg-forwarding")
		// 	framework.ExpectNoError(err)
		// 	defer framework.CleanupTempDir(initialDir, tempDir)
		//
		// 	f := framework.NewDefaultFramework(initialDir + "/bin")
		// 	_ = f.DevPodProviderAdd(ctx, "docker")
		// 	err = f.DevPodProviderUse(context.Background(), "docker")
		// 	framework.ExpectNoError(err)
		//
		// 	ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), tempDir)
		//
		// 	out, err := exec.Command("gpg", "-k").Output()
		// 	if err != nil || len(out) == 0 {
		// 		err = f.SetupGPG(tempDir)
		// 		framework.ExpectNoError(err)
		// 	}
		//
		// 	// Start up devpod workspace
		// 	devpodUpDeadline := time.Now().Add(5 * time.Minute)
		// 	devpodUpCtx, cancel := context.WithDeadline(context.Background(), devpodUpDeadline)
		// 	defer cancel()
		// 	err = f.DevPodUp(devpodUpCtx, tempDir, "--gpg-agent-forwarding")
		// 	framework.ExpectNoError(err)
		//
		// 	devpodSSHDeadline := time.Now().Add(20 * time.Second)
		// 	devpodSSHCtx, cancelSSH := context.WithDeadline(context.Background(), devpodSSHDeadline)
		// 	defer cancelSSH()
		//
		// 	// GPG agent might be not ready, let's try 10 times, 1 second each
		// 	retries := 10
		// 	for retries > 0 {
		// 		err = f.DevPodSSHGpgTestKey(devpodSSHCtx, tempDir)
		// 		if err != nil {
		// 			retries--
		// 			time.Sleep(time.Second)
		// 		} else {
		// 			break
		// 		}
		// 	}
		// 	framework.ExpectNoError(err)
		// })

		ginkgo.It("should start a new workspace with a docker provider (default) and forward a port into it", func() {
			if runtime.GOOS == "windows" {
				return
			}

			tempDir, err := framework.CopyToTempDir("tests/ssh/testdata/forward-test")
			framework.ExpectNoError(err)
			defer framework.CleanupTempDir(initialDir, tempDir)

			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker")
			err = f.DevPodProviderUse(context.Background(), "docker")
			framework.ExpectNoError(err)

			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), tempDir)

			source := rand.NewSource(time.Now().UnixNano())
			rng := rand.New(source)
			port := rng.Intn(1000) + 50000

			devpodUpCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			err = f.DevPodUp(devpodUpCtx, tempDir)
			framework.ExpectNoError(err)

			fmt.Println("Starting pong service on port", port)
			serverCmd := exec.CommandContext(ctx, f.DevpodBinDir+"/"+f.DevpodBinName,
				"ssh", tempDir, "--command",
				"go run /workspaces/"+filepath.Base(tempDir)+"/server.go "+strconv.Itoa(port),
			)
			err = serverCmd.Start()
			framework.ExpectNoError(err)

			fmt.Println("Waiting for server to start")
			time.Sleep(3 * time.Second)

			portForwardCtx, cancelPort := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancelPort()

			fmt.Println("Starting port forwarding for port", port)
			go func() {
				_ = f.DevpodPortTest(portForwardCtx, strconv.Itoa(port), tempDir)
			}()

			fmt.Println("Polling for port", port, "to be accessible")
			address := net.JoinHostPort("localhost", strconv.Itoa(port))
			var out string
			pollReady := false

			for range 30 {
				conn, err := net.DialTimeout("tcp", address, 3*time.Second)
				if err == nil {
					conn.SetReadDeadline(time.Now().Add(2 * time.Second))
					buf := make([]byte, 1024)
					n, readErr := conn.Read(buf)
					conn.Close()
					if readErr == nil && n > 0 {
						out = string(buf[:n])
						pollReady = true
						break
					}
				}
				time.Sleep(2 * time.Second)
			}

			framework.ExpectEqual(pollReady, true, "Port forwarding failed to establish connection")
			framework.ExpectEqual(out, "PONG\n", "Expected PONG response from server")
			fmt.Println("Port forwarding test successful")
		})
	})
})
