package integration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = ginkgo.Describe(
	"devpod provider ssh test suite",
	ginkgo.Label("integration"),
	func() {
		var initialDir string

		ginkgo.BeforeEach(func() {
			var err error
			initialDir, err = os.Getwd()
			framework.ExpectNoError(err)
		})

		ginkgo.It(
			"should setup ssh, add provider, run workspace, test ssh, and cleanup",
			ginkgo.SpecTimeout(framework.GetTimeout()),
			func(ctx context.Context) {
				sshDir := os.Getenv("HOME") + "/.ssh"
				if _, err := os.Stat(sshDir); os.IsNotExist(err) {
					err = os.MkdirAll(sshDir, 0o700)
					framework.ExpectNoError(err)
				}

				homeDir := os.Getenv("HOME")
				sshKeyPath := filepath.Join(homeDir, ".ssh", "id_rsa")
				sshPubKeyPath := filepath.Join(homeDir, ".ssh", "id_rsa.pub")

				_, err := os.Stat(sshKeyPath)
				if err != nil {
					ginkgo.GinkgoWriter.Println("generating ssh keys")
					// #nosec G204 -- ssh-keygen with fixed arguments for test setup
					cmd := exec.CommandContext(
						ctx,
						"ssh-keygen",
						"-q",
						"-t",
						"rsa",
						"-N",
						"",
						"-f",
						sshKeyPath,
					)
					err = cmd.Run()
					framework.ExpectNoError(err)

					// #nosec G204 -- ssh-keygen with fixed arguments for test setup
					cmd = exec.CommandContext(ctx, "ssh-keygen", "-y", "-f", sshKeyPath)
					output, err := cmd.Output()
					framework.ExpectNoError(err)

					err = os.WriteFile(sshPubKeyPath, output, 0o600)
					framework.ExpectNoError(err)
				}

				// #nosec G204 -- ssh-keygen with fixed arguments for test setup
				cmd := exec.CommandContext(ctx, "ssh-keygen", "-y", "-f", sshKeyPath)
				publicKey, err := cmd.Output()
				framework.ExpectNoError(err)

				authorizedKeysPath := filepath.Join(homeDir, ".ssh", "authorized_keys")
				err = os.WriteFile(authorizedKeysPath, publicKey, 0o600)
				framework.ExpectNoError(err)

				f := framework.NewDefaultFramework(initialDir + "/bin")
				// ensure we don't have the ssh provider present
				_ = f.DevPodProviderDelete(ctx, "ssh")

				err = f.DevPodProviderAdd(ctx, "ssh", "-o", "HOST=localhost")
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(func(cleanupCtx context.Context) {
					err = f.DevPodProviderDelete(cleanupCtx, "ssh")
					framework.ExpectNoError(err)
				})

				err = f.DevPodUp(ctx, "tests/integration/testdata/")
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(func(cleanupCtx context.Context) {
					err = f.DevPodWorkspaceDelete(cleanupCtx, "testdata")
					framework.ExpectNoError(err)
				})

				out, err := f.DevPodSSH(ctx, "testdata", "echo test")
				framework.ExpectNoError(err)
				framework.ExpectEqual(out, "test\n")
			},
		)
	},
)
