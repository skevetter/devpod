package up

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = ginkgo.Describe(
	"testing up command with private repos",
	ginkgo.Label("up-private-token"),
	func() {
		var initialDir string

		ginkgo.BeforeEach(func() {
			var err error
			initialDir, err = os.Getwd()
			framework.ExpectNoError(err)
		})

		ginkgo.It("should allow checkout of a private GitRepo", func(ctx context.Context) {
			username := os.Getenv("GH_USERNAME")
			token := os.Getenv("GH_ACCESS_TOKEN")
			if username == "" || token == "" {
				ginkgo.Skip("GH_USERNAME and GH_ACCESS_TOKEN must be set")
			}

			f, err := setupDockerProvider(initialDir+"/bin", "docker")
			framework.ExpectNoError(err)

			// Register credential cleanup before writing to ensure cleanup on any failure
			credentialPath := filepath.Join(os.Getenv("HOME"), ".git-credentials")
			ginkgo.DeferCleanup(func() { _ = os.Remove(credentialPath) })

			// setup git credentials
			err = exec.Command("git", []string{"config", "--global", "credential.helper", "store"}...).
				Run()
			framework.ExpectNoError(err)

			gitCredentialString := []byte("https://" + username + ":" + token + "@github.com")
			err = os.WriteFile(credentialPath, gitCredentialString, 0o600)
			framework.ExpectNoError(err)

			name := "testprivaterepo"
			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, name)

			err = f.DevPodUp(ctx, "https://github.com/"+username+"/test_private_repo.git")
			framework.ExpectNoError(err)

			// verify forwarded credentials by cloning the private repo from within the container
			out, err := f.DevPodSSH(
				ctx,
				name,
				"git clone https://github.com/"+username+"/test_private_repo",
			)
			framework.ExpectNoError(err)
			ginkgo.By(out)
		}, ginkgo.SpecTimeout(framework.GetTimeout()))
	},
)
