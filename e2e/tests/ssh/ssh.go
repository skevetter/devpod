package ssh

import (
	"context"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = ginkgo.Describe("devpod ssh test suite", ginkgo.Label("ssh"), ginkgo.Ordered, func() {
	var initialDir string

	ginkgo.BeforeEach(func() {
		var err error
		initialDir, err = os.Getwd()
		framework.ExpectNoError(err)
	})

	ginkgo.It(
		"should start a new workspace with a docker provider (default) and SSH into it",
		func(ctx context.Context) {
			tempDir, err := framework.CopyToTempDir("tests/ssh/testdata/local-test")
			framework.ExpectNoError(err)

			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker")
			err = f.DevPodProviderUse(ctx, "docker")
			framework.ExpectNoError(err)

			ginkgo.DeferCleanup(func(cleanupCtx context.Context) {
				_ = f.DevPodWorkspaceDelete(cleanupCtx, tempDir)
				framework.CleanupTempDir(initialDir, tempDir)
			})

			// Start up devpod workspace
			devpodUpDeadline := time.Now().Add(5 * time.Minute)
			devpodUpCtx, cancel := context.WithDeadline(ctx, devpodUpDeadline)
			defer cancel()
			err = f.DevPodUp(devpodUpCtx, tempDir)
			framework.ExpectNoError(err)

			devpodSSHDeadline := time.Now().Add(20 * time.Second)
			devpodSSHCtx, cancelSSH := context.WithDeadline(ctx, devpodSSHDeadline)
			defer cancelSSH()
			err = f.DevPodSSHEchoTestString(devpodSSHCtx, tempDir)
			framework.ExpectNoError(err)
		},
	)

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

	ginkgo.It(
		"should set up git SSH signature helper in workspace",
		func(ctx context.Context) {
			if runtime.GOOS == "windows" {
				ginkgo.Skip("skipping on windows")
			}

			tempDir, err := framework.CopyToTempDir("tests/ssh/testdata/ssh-signing")
			framework.ExpectNoError(err)

			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker")
			err = f.DevPodProviderUse(ctx, "docker")
			framework.ExpectNoError(err)

			ginkgo.DeferCleanup(func(cleanupCtx context.Context) {
				_ = f.DevPodWorkspaceDelete(cleanupCtx, tempDir)
				framework.CleanupTempDir(initialDir, tempDir)
			})

			// Generate a temporary SSH key for signing
			sshKeyDir, err := os.MkdirTemp("", "devpod-ssh-signing-test")
			framework.ExpectNoError(err)
			defer func() { _ = os.RemoveAll(sshKeyDir) }()

			keyPath := filepath.Join(sshKeyDir, "id_ed25519")
			// #nosec G204 -- test command with controlled arguments
			err = exec.Command(
				"ssh-keygen", "-t", "ed25519", "-f", keyPath, "-N", "", "-q",
			).Run()
			framework.ExpectNoError(err)

			// Start workspace with git-ssh-signing-key flag
			devpodUpCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			defer cancel()
			err = f.DevPodUp(devpodUpCtx, tempDir,
				"--git-ssh-signing-key", keyPath+".pub",
			)
			framework.ExpectNoError(err)

			sshCtx, cancelSSH := context.WithTimeout(ctx, 20*time.Second)
			defer cancelSSH()

			// Verify the helper script was installed
			out, err := f.DevPodSSH(sshCtx, tempDir,
				"test -x /usr/local/bin/devpod-ssh-signature && echo EXISTS",
			)
			framework.ExpectNoError(err)
			gomega.Expect(strings.TrimSpace(out)).To(
				gomega.Equal("EXISTS"),
				"devpod-ssh-signature helper script should be installed and executable",
			)

			// Verify git config has the SSH signing program set
			out, err = f.DevPodSSH(sshCtx, tempDir,
				"git config --global gpg.ssh.program",
			)
			framework.ExpectNoError(err)
			gomega.Expect(strings.TrimSpace(out)).To(
				gomega.Equal("devpod-ssh-signature"),
				"git gpg.ssh.program should be set to devpod-ssh-signature",
			)

			// Verify git config has gpg format set to ssh
			out, err = f.DevPodSSH(sshCtx, tempDir,
				"git config --global gpg.format",
			)
			framework.ExpectNoError(err)
			gomega.Expect(strings.TrimSpace(out)).To(
				gomega.Equal("ssh"),
				"git gpg.format should be set to ssh",
			)
		},
	)

	ginkgo.It(
		"should start a new workspace with a docker provider (default) and forward a port into it",
		func(ctx context.Context) {
			if runtime.GOOS == "windows" {
				ginkgo.Skip("skipping on windows")
			}

			tempDir, err := framework.CopyToTempDir("tests/ssh/testdata/forward-test")
			framework.ExpectNoError(err)

			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker")
			err = f.DevPodProviderUse(ctx, "docker")
			framework.ExpectNoError(err)

			ginkgo.DeferCleanup(func(cleanupCtx context.Context) {
				err = f.DevPodWorkspaceDelete(cleanupCtx, tempDir)
				framework.ExpectNoError(err)
				framework.CleanupTempDir(initialDir, tempDir)
			})

			source := rand.NewSource(time.Now().UnixNano())
			rng := rand.New(source) // #nosec G404 -- weak random is fine for test port selection
			port := rng.Intn(1000) + 50000

			devpodUpCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			defer cancel()
			err = f.DevPodUp(devpodUpCtx, tempDir)
			framework.ExpectNoError(err)

			ginkgo.GinkgoWriter.Println("Starting pong service on port", port)
			// Create a cancelable context for the server command
			serverCtx, serverCancel := context.WithCancel(ctx)
			defer serverCancel()
			// #nosec G204 -- test command with controlled arguments
			serverCmd := exec.CommandContext(serverCtx, f.DevpodBinDir+"/"+f.DevpodBinName,
				"ssh", tempDir, "--command",
				"go run /workspaces/"+filepath.Base(tempDir)+"/server.go "+strconv.Itoa(port),
			)
			err = serverCmd.Start()
			framework.ExpectNoError(err)
			go func() {
				_ = serverCmd.Wait()
			}()

			ginkgo.GinkgoWriter.Println("Waiting for server to start")
			time.Sleep(3 * time.Second)

			portForwardCtx, cancelPort := context.WithTimeout(
				ctx,
				60*time.Second,
			)
			defer cancelPort()

			ginkgo.GinkgoWriter.Println("Starting port forwarding for port", port)
			go func() {
				_ = f.DevpodPortTest(portForwardCtx, strconv.Itoa(port), tempDir)
			}()

			ginkgo.GinkgoWriter.Println("Polling for port", port, "to be accessible")
			address := net.JoinHostPort("localhost", strconv.Itoa(port))

			var out string
			gomega.Eventually(func() string {
				conn, err := net.DialTimeout("tcp", address, 3*time.Second)
				if err == nil {
					_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
					buf := make([]byte, 1024)
					n, readErr := conn.Read(buf)
					_ = conn.Close()
					if readErr == nil && n > 0 {
						out = string(buf[:n])
						return out
					}
				}
				return ""
			}, 60*time.Second, 2*time.Second).Should(
				gomega.Equal("PONG\n"),
				"Port forwarding failed to establish connection",
			)
			ginkgo.GinkgoWriter.Println("Port forwarding test successful")
		},
	)
})
