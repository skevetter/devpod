package up

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = DevPodDescribe("devpod up private repo test suite", func() {
	ginkgo.Context("testing up command with private repos", ginkgo.Label("up", "up-private-token"), func() {
		var initialDir string

		ginkgo.BeforeEach(func() {
			var err error
			initialDir, err = os.Getwd()
			framework.ExpectNoError(err)
		})

		ginkgo.It("should allow checkout of a private GitRepo", func() {
			username := os.Getenv("GH_USERNAME")
			token := os.Getenv("GH_ACCESS_TOKEN")

			ctx := context.Background()
			f, err := setupDockerProvider(initialDir+"/bin", "docker")
			framework.ExpectNoError(err)

			// setup git credentials
			err = exec.Command("git", []string{"config", "--global", "credential.helper", "store"}...).Run()
			framework.ExpectNoError(err)

			gitCredentialString := []byte("https://" + username + ":" + token + "@github.com")
			err = os.WriteFile(
				filepath.Join(os.Getenv("HOME"), ".git-credentials"),
				gitCredentialString, 0o644)
			framework.ExpectNoError(err)
			defer func() { _ = os.Remove(filepath.Join(os.Getenv("HOME"), ".git-credentials")) }()

			name := "testprivaterepo"
			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), name)

			err = f.DevPodUp(ctx, "https://github.com/"+username+"/test_private_repo.git")
			framework.ExpectNoError(err)

			// verify forwarded credentials by cloning the private repo from within the container
			out, err := f.DevPodSSH(ctx, name, "git clone https://github.com/"+username+"/test_private_repo")
			framework.ExpectNoError(err)
			fmt.Println(out)
		})
	})
})
