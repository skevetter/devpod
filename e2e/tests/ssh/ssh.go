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

const osWindows = "windows"

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
	// 	if runtime.GOOS == osWindows {
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
		"should set up git SSH signature helper and sign a commit",
		ginkgo.SpecTimeout(7*time.Minute),
		func(ctx ginkgo.SpecContext) {
			if runtime.GOOS == osWindows {
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

			agentOut, err := exec.Command("ssh-agent", "-s").Output()
			framework.ExpectNoError(err)
			t := ginkgo.GinkgoT()
			var agentPID string
			for line := range strings.SplitSeq(string(agentOut), "\n") {
				for _, prefix := range []string{"SSH_AUTH_SOCK=", "SSH_AGENT_PID="} {
					if _, after, ok := strings.Cut(line, prefix); ok {
						val := after
						if semi := strings.Index(val, ";"); semi >= 0 {
							val = val[:semi]
						}
						key := prefix[:len(prefix)-1]
						if key == "SSH_AGENT_PID" {
							agentPID = val
						}
						t.Setenv(key, val)
					}
				}
			}
			ginkgo.DeferCleanup(func(_ context.Context) {
				if agentPID != "" {
					// #nosec G204 -- controlled pid from ssh-agent we started
					_ = exec.Command("kill", agentPID).Run()
				}
			})

			// #nosec G204 -- test command with controlled arguments
			err = exec.Command("ssh-add", keyPath).Run()
			framework.ExpectNoError(err)

			// Start workspace with git-ssh-signing-key flag
			err = f.DevPodUp(ctx, tempDir, "--git-ssh-signing-key", keyPath+".pub")
			framework.ExpectNoError(err)

			// Verify helper installation, git config, and a signed commit
			// in a single SSH session with --start-services so the
			// credentials server tunnel is active. The helper is installed
			// asynchronously by the credentials server, so retry briefly.
			commitCmd := strings.Join([]string{
				"for i in $(seq 1 30); do test -x /usr/local/bin/devpod-ssh-signature && break; sleep 1; done",
				"test -x /usr/local/bin/devpod-ssh-signature",
				"test \"$(git config --global gpg.ssh.program)\" = devpod-ssh-signature",
				"test \"$(git config --global gpg.format)\" = ssh",
				"cd /tmp",
				"git init test-sign-repo",
				"cd test-sign-repo",
				"git config user.name 'Test User'",
				"git config user.email 'test@example.com'",
				"git config commit.gpgsign true",
				"echo test > testfile",
				"git add testfile",
				"git commit -m 'signed test commit' 2>&1",
				"ssh-add -L | head -1 > /tmp/test_signing_key.pub",
				"git config --global user.signingkey /tmp/test_signing_key.pub",
				"echo world >> file.txt",
				"git add file.txt",
				"git commit -m 'signed commit with file path key' 2>&1",
			}, " && ")

			// The signing key must be passed on each SSH invocation so the
			// credentials server can configure the helper inside the container.
			sshBase := []string{
				"ssh",
				"--agent-forwarding",
				"--start-services",
				"--git-ssh-signing-key", keyPath + ".pub",
				tempDir,
			}

			stdout, stderr, err := f.ExecCommandCapture(ctx,
				append(sshBase, "--command", commitCmd),
			)
			ginkgo.GinkgoWriter.Printf("commit stdout: %s\n", stdout)
			ginkgo.GinkgoWriter.Printf("commit stderr: %s\n", stderr)
			framework.ExpectNoError(err)

			gomega.Expect(stdout).To(
				gomega.ContainSubstring("signed test commit"),
				"git commit should succeed with the signed test commit message",
			)

			// Verify the commit is signed with a valid SSH signature.
			pubKeyBytes, err := os.ReadFile(
				keyPath + ".pub",
			) // #nosec G304 -- test file with controlled path
			framework.ExpectNoError(err)
			pubKey := strings.TrimSpace(string(pubKeyBytes))

			verifyCmd := strings.Join([]string{
				"cd /tmp/test-sign-repo",
				"echo 'test@example.com " + pubKey + "' > /tmp/allowed_signers",
				"git config gpg.ssh.allowedSignersFile /tmp/allowed_signers",
				"git verify-commit HEAD 2>&1",
			}, " && ")

			stdout, stderr, err = f.ExecCommandCapture(ctx,
				append(sshBase, "--command", verifyCmd),
			)
			ginkgo.GinkgoWriter.Printf("verify stdout: %s\n", stdout)
			ginkgo.GinkgoWriter.Printf("verify stderr: %s\n", stderr)
			framework.ExpectNoError(err)

			// git verify-commit writes signature details to stderr
			combined := stdout + stderr
			gomega.Expect(combined).To(
				gomega.ContainSubstring("Good"),
				"git verify-commit should report a good SSH signature",
			)

			// And confirm the signature log shows the correct principal
			logCmd := "cd /tmp/test-sign-repo && git log --show-signature -1 2>&1"
			stdout, stderr, err = f.ExecCommandCapture(ctx,
				append(sshBase, "--command", logCmd),
			)
			ginkgo.GinkgoWriter.Printf("log stdout: %s\n", stdout)
			ginkgo.GinkgoWriter.Printf("log stderr: %s\n", stderr)
			framework.ExpectNoError(err)

			combined = stdout + stderr
			gomega.Expect(combined).To(
				gomega.ContainSubstring("Good"),
				"git log --show-signature should report a good signature",
			)
			gomega.Expect(combined).To(
				gomega.ContainSubstring("test@example.com"),
				"signature should be associated with the test email principal",
			)
		},
	)

	ginkgo.It(
		"should not install git SSH signature helper when signing key is not provided",
		ginkgo.SpecTimeout(5*time.Minute),
		func(ctx ginkgo.SpecContext) {
			if runtime.GOOS == osWindows {
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

			// Start workspace WITHOUT --git-ssh-signing-key
			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			// Verify the helper script was NOT installed
			out, err := f.DevPodSSH(ctx, tempDir,
				"test -x /usr/local/bin/devpod-ssh-signature && echo EXISTS || echo MISSING",
			)
			framework.ExpectNoError(err)
			gomega.Expect(strings.TrimSpace(out)).To(
				gomega.Equal("MISSING"),
				"devpod-ssh-signature helper should not be installed without --git-ssh-signing-key",
			)

			// Verify git config was NOT set for SSH signing
			out, err = f.DevPodSSH(ctx, tempDir,
				"git config --global gpg.ssh.program || echo UNSET",
			)
			framework.ExpectNoError(err)
			gomega.Expect(strings.TrimSpace(out)).To(
				gomega.Equal("UNSET"),
				"gpg.ssh.program should not be configured without --git-ssh-signing-key",
			)
		},
	)

	ginkgo.It(
		"should surface clear error when SSH signing fails",
		ginkgo.SpecTimeout(7*time.Minute),
		func(ctx ginkgo.SpecContext) {
			if runtime.GOOS == osWindows {
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

			// Generate a key but do NOT add it to the ssh-agent so signing will fail
			sshKeyDir, err := os.MkdirTemp("", "devpod-ssh-signing-err-test")
			framework.ExpectNoError(err)
			defer func() { _ = os.RemoveAll(sshKeyDir) }()

			keyPath := filepath.Join(sshKeyDir, "id_ed25519")
			// #nosec G204 -- test command with controlled arguments
			err = exec.Command(
				"ssh-keygen", "-t", "ed25519", "-f", keyPath, "-N", "", "-q",
			).Run()
			framework.ExpectNoError(err)

			// Start workspace with signing key
			err = f.DevPodUp(ctx, tempDir, "--git-ssh-signing-key", keyPath+".pub")
			framework.ExpectNoError(err)

			// Attempt a signed commit — this should fail because the key
			// is not in the agent, but the error must be human-readable.
			commitCmd := strings.Join([]string{
				"cd /tmp",
				"git init test-sign-err-repo",
				"cd test-sign-err-repo",
				"git config user.name 'Test User'",
				"git config user.email 'test@example.com'",
				"git config commit.gpgsign true",
				"echo test > testfile",
				"git add testfile",
				"git commit -m 'signed test commit' 2>&1",
			}, " && ")

			stdout, stderr, err := f.ExecCommandCapture(ctx, []string{
				"ssh",
				"--agent-forwarding",
				"--start-services",
				tempDir,
				"--command", commitCmd,
			})
			ginkgo.GinkgoWriter.Printf("error commit stdout: %s\n", stdout)
			ginkgo.GinkgoWriter.Printf("error commit stderr: %s\n", stderr)

			// The commit should fail
			combined := stdout + stderr
			if err != nil {
				combined += err.Error()
			}

			// The error must NOT contain JSON decode artifacts
			gomega.Expect(combined).NotTo(
				gomega.ContainSubstring("invalid character"),
				"error should not contain JSON parse errors — error messages must be human-readable",
			)
		},
	)

	ginkgo.It(
		"should start a new workspace with a docker provider (default) and forward a port into it",
		func(ctx context.Context) {
			if runtime.GOOS == osWindows {
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
