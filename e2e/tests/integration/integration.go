package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = ginkgo.Describe("[integration]: devpod provider ssh test suite", ginkgo.Ordered, func() {
	ginkgo.Context("testing provider integration", ginkgo.Label("integration"), ginkgo.Ordered, func() {
		var initialDir string

		ginkgo.BeforeEach(func() {
			var err error
			initialDir, err = os.Getwd()
			framework.ExpectNoError(err)
		})

		ginkgo.It("should generate ssh keypairs", func(ctx context.Context) {
			sshDir := os.Getenv("HOME") + "/.ssh"
			if _, err := os.Stat(sshDir); os.IsNotExist(err) {
				err = os.MkdirAll(sshDir, 0700)
				framework.ExpectNoError(err)
			}

			homeDir := os.Getenv("HOME")
			sshKeyPath := filepath.Join(homeDir, ".ssh", "id_rsa")
			sshPubKeyPath := filepath.Join(homeDir, ".ssh", "id_rsa.pub")

			_, err := os.Stat(sshKeyPath)
			if err != nil {
				fmt.Println("generating ssh keys")
				// #nosec G204 -- ssh-keygen with fixed arguments for test setup
				cmd := exec.CommandContext(ctx, "ssh-keygen", "-q", "-t", "rsa", "-N", "", "-f", sshKeyPath)
				err = cmd.Run()
				framework.ExpectNoError(err)

				// #nosec G204 -- ssh-keygen with fixed arguments for test setup
				cmd = exec.CommandContext(ctx, "ssh-keygen", "-y", "-f", sshKeyPath)
				output, err := cmd.Output()
				framework.ExpectNoError(err)

				err = os.WriteFile(sshPubKeyPath, output, 0600)
				framework.ExpectNoError(err)
			}

			// #nosec G204 -- ssh-keygen with fixed arguments for test setup
			cmd := exec.CommandContext(ctx, "ssh-keygen", "-y", "-f", sshKeyPath)
			publicKey, err := cmd.Output()
			framework.ExpectNoError(err)

			authorizedKeysPath := filepath.Join(homeDir, ".ssh", "authorized_keys")
			_, err = os.Stat(authorizedKeysPath)
			if err != nil {
				err = os.WriteFile(authorizedKeysPath, publicKey, 0600)
				framework.ExpectNoError(err)
			} else {
				f, err := os.OpenFile(os.Getenv("HOME")+"/.ssh/authorized_keys",
					os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
				framework.ExpectNoError(err)

				defer func() { _ = f.Close() }()
				_, err = f.Write(publicKey)
				framework.ExpectNoError(err)
			}
		})

		ginkgo.It("should add provider to devpod", func(ctx context.Context) {
			f := framework.NewDefaultFramework(initialDir + "/bin")
			// ensure we don't have the ssh provider present
			err := f.DevPodProviderDelete(ctx, "ssh")
			if err != nil {
				fmt.Println("warning: " + err.Error())
			}

			err = f.DevPodProviderAdd(ctx, "ssh", "-o", "HOST=localhost")
			framework.ExpectNoError(err)
		})

		ginkgo.It("should run devpod up", func(ctx context.Context) {
			f := framework.NewDefaultFramework(initialDir + "/bin")
			err := f.DevPodUp(ctx, "tests/integration/testdata/")
			framework.ExpectNoError(err)
		})

		ginkgo.It("should run commands to workspace via ssh", func(ctx context.Context) {
			f := framework.NewDefaultFramework(initialDir + "/bin")
			out, err := f.DevPodSSH(ctx, "testdata", "echo test")
			framework.ExpectNoError(err)
			framework.ExpectEqual(out, "test\n")
		})

		ginkgo.It("should cleanup devpod workspace", func(ctx context.Context) {
			f := framework.NewDefaultFramework(initialDir + "/bin")
			err := f.DevPodWorkspaceDelete(ctx, "testdata")
			framework.ExpectNoError(err)
		})
	})
})
