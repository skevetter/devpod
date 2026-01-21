//go:build windows
package up

import (
	"context"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega/ghttp"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = ginkgo.Describe("devpod up docker features test suite", ginkgo.Label("up-features", "wsl"), func() {
	var initialDir string

	ginkgo.BeforeEach(func() {
		var err error
		initialDir, err = os.Getwd()
		framework.ExpectNoError(err)
	})

	ginkgo.It("should use http headers to download feature", func(ctx context.Context) {
		server := ghttp.NewServer()

		tempDir, err := framework.CopyToTempDir("tests/up-features/testdata/docker-features-http-headers")
		framework.ExpectNoError(err)

		featureArchiveFilePath := path.Join(tempDir, "devcontainer-feature-hello.tgz")
		featureFiles := []string{path.Join(tempDir, "devcontainer-feature.json"), path.Join(tempDir, "install.sh")}
		err = createTarGzArchive(featureArchiveFilePath, featureFiles)
		framework.ExpectNoError(err)

		devContainerFileBuf, err := os.ReadFile(path.Join(tempDir, ".devcontainer.json"))
		framework.ExpectNoError(err)

		output := strings.ReplaceAll(string(devContainerFileBuf), "#{server_url}", server.URL())
		err = os.WriteFile(path.Join(tempDir, ".devcontainer.json"), []byte(output), 0644)
		framework.ExpectNoError(err)

		ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)
		ginkgo.DeferCleanup(server.Close)

		respHeader := http.Header{}
		respHeader.Set("Content-Disposition", "attachment; filename=devcontainer-feature-hello.tgz")

		featureArchiveFileBuf, err := os.ReadFile(featureArchiveFilePath)
		framework.ExpectNoError(err)

		f := framework.NewDefaultFramework(initialDir + "/bin")
		server.AppendHandlers(
			ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", "/devcontainer-feature-hello.tgz"),
				ghttp.VerifyHeaderKV("Foo-Header", "Foo"),
				ghttp.RespondWith(http.StatusOK, featureArchiveFileBuf, respHeader),
			),
		)

		_ = f.DevPodProviderDelete(ctx, "docker")
		err = f.DevPodProviderAdd(ctx, "docker")
		framework.ExpectNoError(err)
		err = f.DevPodProviderUse(ctx, "docker")
		framework.ExpectNoError(err)

		ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), tempDir)

		// Wait for devpod workspace to come online (deadline: 30s)
		err = f.DevPodUp(ctx, tempDir)
		framework.ExpectNoError(err)
		server.Close()
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("ensure dependencies installed via features are accessible in lifecycle hooks", func(ctx context.Context) {
		f, err := setupDockerProvider(initialDir+"/bin", "docker")
		framework.ExpectNoError(err)

		tempDir, err := framework.CopyToTempDir("tests/up-features/testdata/docker-features-lifecycle-hooks")
		framework.ExpectNoError(err)
		ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

		workspaceName := filepath.Base(tempDir)
		ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), workspaceName)

		// Wait for devpod workspace to come online (deadline: 30s)
		err = f.DevPodUp(ctx, tempDir, "--debug")
		framework.ExpectNoError(err)
	}, ginkgo.SpecTimeout(framework.GetTimeout()))
})
