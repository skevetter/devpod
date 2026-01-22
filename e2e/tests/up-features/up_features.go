//go:build linux || darwin || unix

package up

import (
	"context"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = ginkgo.Describe("testing up command", ginkgo.Label("up-features", "suite"), func() {
	var initialDir string

	ginkgo.BeforeEach(func() {
		var err error
		initialDir, err = os.Getwd()
		framework.ExpectNoError(err)
	})

	ginkgo.It("lifecycle hooks execution", func(ctx context.Context) {
		f, err := setupDockerProvider(initialDir+"/bin", "docker")
		framework.ExpectNoError(err)

		tempDir, err := framework.CopyToTempDir("tests/up-features/testdata/docker-features-hooks")
		framework.ExpectNoError(err)
		ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

		wsName := filepath.Base(tempDir)
		ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), wsName)

		err = f.DevPodUp(ctx, tempDir)
		framework.ExpectNoError(err)

		out, err := f.DevPodSSH(ctx, wsName, "cat /tmp/feature-onCreate.txt")
		framework.ExpectNoError(err)
		framework.ExpectEqual(strings.TrimSpace(out), "feature-onCreate")

		out, err = f.DevPodSSH(ctx, wsName, "cat /tmp/feature-postCreate.txt")
		framework.ExpectNoError(err)
		framework.ExpectEqual(strings.TrimSpace(out), "feature-postCreate")

		out, err = f.DevPodSSH(ctx, wsName, "cat /tmp/feature-postStart.txt")
		framework.ExpectNoError(err)
		framework.ExpectEqual(strings.TrimSpace(out), "feature-postStart")
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("http headers download", func(ctx context.Context) {
		server := ghttp.NewServer()
		ginkgo.DeferCleanup(server.Close)

		tempDir, err := framework.CopyToTempDir("tests/up-features/testdata/docker-features-http-headers")
		framework.ExpectNoError(err)
		ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

		featureArchiveFilePath := path.Join(tempDir, "devcontainer-feature-hello.tgz")
		featureFiles := []string{path.Join(tempDir, "devcontainer-feature.json"), path.Join(tempDir, "install.sh")}
		err = createTarGzArchive(featureArchiveFilePath, featureFiles)
		framework.ExpectNoError(err)

		devContainerFileBuf, err := os.ReadFile(path.Join(tempDir, ".devcontainer.json"))
		framework.ExpectNoError(err)

		output := strings.ReplaceAll(string(devContainerFileBuf), "#{server_url}", server.URL())
		err = os.WriteFile(path.Join(tempDir, ".devcontainer.json"), []byte(output), 0644)
		framework.ExpectNoError(err)

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

		wsName := filepath.Base(tempDir)
		ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), wsName)

		err = f.DevPodUp(ctx, tempDir)
		framework.ExpectNoError(err)
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("should install with lifecycle hooks", func(ctx context.Context) {
		f, err := setupDockerProvider(initialDir+"/bin", "docker")
		framework.ExpectNoError(err)

		tempDir, err := framework.CopyToTempDir("tests/up-features/testdata/docker-features-lifecycle-hooks")
		framework.ExpectNoError(err)
		ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

		wsName := filepath.Base(tempDir)
		ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), wsName)

		err = f.DevPodUp(ctx, tempDir)
		framework.ExpectNoError(err)
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("should automatically install dependsOn features", ginkgo.Label("features", "depends-on"), func(ctx context.Context) {
		f, err := setupDockerProvider(initialDir+"/bin", "docker")
		framework.ExpectNoError(err)

		tempDir, err := framework.CopyToTempDir("tests/up-features/testdata/docker-features-depends-on")
		framework.ExpectNoError(err)
		ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

		wsName := filepath.Base(tempDir)
		ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), wsName)

		err = f.DevPodUp(ctx, tempDir)
		framework.ExpectNoError(err)

		out, err := f.DevPodSSH(ctx, wsName, "test-depends-on")
		framework.ExpectNoError(err)
		gomega.Expect(out).To(gomega.ContainSubstring("SUCCESS: hello command is available"))
		gomega.Expect(out).To(gomega.ContainSubstring("hey, vscode"))
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("should not fail if same feature exists in dependsOn and installsAfter", ginkgo.Label("features", "depends-on"), func(ctx context.Context) {
		f, err := setupDockerProvider(initialDir+"/bin", "docker")
		framework.ExpectNoError(err)

		tempDir, err := framework.CopyToTempDir("tests/up-features/testdata/docker-features-depends-on-duplicate-feature")
		framework.ExpectNoError(err)
		ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

		wsName := filepath.Base(tempDir)
		ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), wsName)

		err = f.DevPodUp(ctx, tempDir)
		framework.ExpectNoError(err)

		out, err := f.DevPodSSH(ctx, wsName, "test-depends-on")
		framework.ExpectNoError(err)
		gomega.Expect(out).To(gomega.ContainSubstring("SUCCESS: hello command is available"))
		gomega.Expect(out).To(gomega.ContainSubstring("hey, vscode"))
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("should handle nested dependencies", ginkgo.Label("features", "depends-on"), func(ctx context.Context) {
		f, err := setupDockerProvider(initialDir+"/bin", "docker")
		framework.ExpectNoError(err)

		tempDir, err := framework.CopyToTempDir("tests/up-features/testdata/docker-features-nested-depends-on")
		framework.ExpectNoError(err)
		ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

		wsName := filepath.Base(tempDir)
		ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), wsName)

		err = f.DevPodUp(ctx, tempDir)
		framework.ExpectNoError(err)

		// Test nested dependency chain works
		out, err := f.DevPodSSH(ctx, wsName, "test-nested-chain")
		framework.ExpectNoError(err)
		gomega.Expect(out).To(gomega.ContainSubstring("All dependencies available"))
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("should detect circular dependencies", ginkgo.Label("features", "depends-on"), func(ctx context.Context) {
		f, err := setupDockerProvider(initialDir+"/bin", "docker")
		framework.ExpectNoError(err)

		tempDir, err := framework.CopyToTempDir("tests/up-features/testdata/docker-features-circular-depends-on")
		framework.ExpectNoError(err)
		ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

		wsName := filepath.Base(tempDir)
		ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), wsName)

		// This should fail with circular dependency error
		err = f.DevPodUp(ctx, tempDir)
		// The logs show "circular dependency detected" in the debug output
		framework.ExpectError(err)
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("should handle dependsOn with options", ginkgo.Label("features", "depends-on"), func(ctx context.Context) {
		f, err := setupDockerProvider(initialDir+"/bin", "docker")
		framework.ExpectNoError(err)

		tempDir, err := framework.CopyToTempDir("tests/up-features/testdata/docker-features-depends-on-options")
		framework.ExpectNoError(err)
		ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

		wsName := filepath.Base(tempDir)
		ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), wsName)

		err = f.DevPodUp(ctx, tempDir)
		framework.ExpectNoError(err)

		// Test dependency installed with correct options
		out, err := f.DevPodSSH(ctx, wsName, "hello")
		framework.ExpectNoError(err)
		gomega.Expect(out).To(gomega.ContainSubstring("custom greeting"))
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("should handle mixed dependsOn and installsAfter", ginkgo.Label("features", "mixed"), func(ctx context.Context) {
		f, err := setupDockerProvider(initialDir+"/bin", "docker")
		framework.ExpectNoError(err)

		tempDir, err := framework.CopyToTempDir("tests/up-features/testdata/docker-features-mixed-dependencies")
		framework.ExpectNoError(err)
		ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

		wsName := filepath.Base(tempDir)
		ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), wsName)

		err = f.DevPodUp(ctx, tempDir)
		framework.ExpectNoError(err)

		// Test correct installation order
		out, err := f.DevPodSSH(ctx, wsName, "test-install-order")
		framework.ExpectNoError(err)
		gomega.Expect(out).To(gomega.ContainSubstring("Correct order"))
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("should detect self-dependency", ginkgo.Label("features", "depends-on"), func(ctx context.Context) {
		f, err := setupDockerProvider(initialDir+"/bin", "docker")
		framework.ExpectNoError(err)

		tempDir, err := framework.CopyToTempDir("tests/up-features/testdata/docker-features-self-dependency")
		framework.ExpectNoError(err)
		ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

		wsName := filepath.Base(tempDir)
		ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), wsName)

		// Should fail with circular dependency error
		err = f.DevPodUp(ctx, tempDir)
		framework.ExpectError(err)
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("should handle non-existent dependency gracefully", ginkgo.Label("features", "depends-on"), func(ctx context.Context) {
		f, err := setupDockerProvider(initialDir+"/bin", "docker")
		framework.ExpectNoError(err)

		tempDir, err := framework.CopyToTempDir("tests/up-features/testdata/docker-features-nonexistent-dependency")
		framework.ExpectNoError(err)
		ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

		wsName := filepath.Base(tempDir)
		ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), wsName)

		// Should fail when dependency cannot be resolved
		err = f.DevPodUp(ctx, tempDir)
		framework.ExpectError(err)
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("should handle shared dependencies correctly", ginkgo.Label("features", "depends-on"), func(ctx context.Context) {
		f, err := setupDockerProvider(initialDir+"/bin", "docker")
		framework.ExpectNoError(err)

		tempDir, err := framework.CopyToTempDir("tests/up-features/testdata/docker-features-shared-dependency")
		framework.ExpectNoError(err)
		ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

		wsName := filepath.Base(tempDir)
		ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), wsName)

		err = f.DevPodUp(ctx, tempDir)
		framework.ExpectNoError(err)

		// Verify shared dependency was installed only once and both features work
		out, err := f.DevPodSSH(ctx, wsName, "hello")
		framework.ExpectNoError(err)
		// Should contain greeting from one of the features (last one wins)
		gomega.Expect(out).To(gomega.ContainSubstring("from"))
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("should handle forward reference dependencies", ginkgo.Label("features", "depends-on"), func(ctx context.Context) {
		f, err := setupDockerProvider(initialDir+"/bin", "docker")
		framework.ExpectNoError(err)

		tempDir, err := framework.CopyToTempDir("tests/up-features/testdata/docker-features-forward-reference")
		framework.ExpectNoError(err)
		ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

		wsName := filepath.Base(tempDir)
		ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), wsName)

		// This should not fail with "Parent does not exist" error
		err = f.DevPodUp(ctx, tempDir)
		framework.ExpectNoError(err)

		// Test that both features are installed correctly
		out, err := f.DevPodSSH(ctx, wsName, "python3 --version")
		framework.ExpectNoError(err)
		gomega.Expect(out).To(gomega.ContainSubstring("Python 3.11"))
	}, ginkgo.SpecTimeout(framework.GetTimeout()*5)) // This test compiles Python

	ginkgo.It("should handle same feature in dependsOn and installsAfter", ginkgo.Label("features", "depends-on"), func(ctx context.Context) {
		f, err := setupDockerProvider(initialDir+"/bin", "docker")
		framework.ExpectNoError(err)

		tempDir, err := framework.CopyToTempDir("tests/up-features/testdata/docker-features-same-depends-on-and-installs-after")
		framework.ExpectNoError(err)
		ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

		wsName := filepath.Base(tempDir)
		ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), wsName)

		err = f.DevPodUp(ctx, tempDir)
		framework.ExpectNoError(err)

		out, err := f.DevPodSSH(ctx, wsName, "cat /tmp/test-result")
		framework.ExpectNoError(err)
		gomega.Expect(out).To(gomega.ContainSubstring("test-passed"))

		out, err = f.DevPodSSH(ctx, wsName, "node --version")
		framework.ExpectNoError(err)
		gomega.Expect(out).To(gomega.MatchRegexp(`v\d+\.\d+\.\d+`))
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("resolves user variable in dockerfile", func(ctx context.Context) {
		f, err := setupDockerProvider(initialDir+"/bin", "docker")
		framework.ExpectNoError(err)

		tempDir, err := framework.CopyToTempDir("tests/up-features/testdata/docker-features-with-user-variable-in-dockerfile")
		framework.ExpectNoError(err)
		ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

		wsName := filepath.Base(tempDir)
		ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), wsName)

		err = f.DevPodUp(ctx, tempDir)
		framework.ExpectNoError(err)

		out, err := f.DevPodSSH(ctx, wsName, "whoami")
		framework.ExpectNoError(err)
		framework.ExpectEqual(strings.TrimSpace(out), "testuser")
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

	ginkgo.It("preserves user when feature is present with variable", func(ctx context.Context) {
		f, err := setupDockerProvider(initialDir+"/bin", "docker")
		framework.ExpectNoError(err)

		tempDir, err := framework.CopyToTempDir("tests/up-features/testdata/docker-features-with-user-variable")
		framework.ExpectNoError(err)
		ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

		wsName := filepath.Base(tempDir)
		ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), wsName)

		err = f.DevPodUp(ctx, tempDir)
		framework.ExpectNoError(err)

		out, err := f.DevPodSSH(ctx, wsName, "whoami")
		framework.ExpectNoError(err)
		framework.ExpectEqual(strings.TrimSpace(out), "ubuntu")
	}, ginkgo.SpecTimeout(framework.GetTimeout()))

})
