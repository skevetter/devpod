package up

import (
	"context"
	"os"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = ginkgo.Describe(
	"testing up command that handles workspace errors",
	ginkgo.Label("up-handle-errors"),
	func() {
		var initialDir string

		ginkgo.BeforeEach(func() {
			var err error
			initialDir, err = os.Getwd()
			framework.ExpectNoError(err)
		})

		ginkgo.It(
			"make sure devpod output is correct and log-output works correctly",
			func(ctx context.Context) {
				f := framework.NewDefaultFramework(initialDir + "/bin")
				tempDir, err := framework.CopyToTempDir("tests/up/testdata/docker")
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

				err = f.DevPodProviderAdd(ctx, "docker", "--name", "test-docker")
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(func(cleanupCtx context.Context) {
					err := f.DevPodProviderDelete(cleanupCtx, "test-docker")
					framework.ExpectNoError(err)
				})

				err = f.DevPodProviderUse(
					ctx,
					"test-docker",
					"-o",
					"DOCKER_PATH=abc",
					"--skip-init",
				)
				framework.ExpectNoError(err)

				// Wait for devpod workspace to come online
				stdout, stderr, err := f.DevPodUpStreams(ctx, tempDir, "--log-output=json")
				deleteErr := f.DevPodWorkspaceDelete(ctx, tempDir, "--force")
				framework.ExpectNoError(deleteErr)
				framework.ExpectError(err, "expected error")
				framework.ExpectNoError(verifyLogStream(strings.NewReader(stdout)))
				framework.ExpectNoError(verifyLogStream(strings.NewReader(stderr)))
				framework.ExpectNoError(
					findMessage(
						strings.NewReader(stderr),
						"exec: \"abc\": executable file not found in $PATH",
					),
				)
			},
			ginkgo.SpecTimeout(framework.GetTimeout()),
		)

		ginkgo.It(
			"forwards the final lifecycle hook log line before the agent exits",
			func(ctx context.Context) {
				f, err := setupDockerProvider(initialDir+"/bin", "docker")
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(func(cleanupCtx context.Context) {
					_ = f.DevPodProviderDelete(cleanupCtx, "docker")
				})

				tempDir, err := framework.CopyToTempDir(
					"tests/up/testdata/docker-lifecycle-stderr",
				)
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)
				ginkgo.DeferCleanup(func(cleanupCtx context.Context) {
					_ = f.DevPodWorkspaceDelete(cleanupCtx, tempDir, "--force")
				})

				// The postCreateCommand prints a marker to stderr and exits
				// non-zero. The agent forwards lifecycle hook output over the
				// tunnel asynchronously, so on failure it must flush the final
				// queued line before tearing down. Regression guard for the
				// dropped-last-line bug.
				stdout, stderr, err := f.DevPodUpStreams(ctx, tempDir, "--log-output=json")
				framework.ExpectError(err, "expected lifecycle hook failure")
				framework.ExpectNoError(
					findMessage(
						strings.NewReader(stdout+stderr),
						"DEVPOD_LIFECYCLE_FLUSH_MARKER",
					),
				)
			},
			ginkgo.SpecTimeout(framework.GetTimeout()),
		)

		ginkgo.It(
			"ensure workspace cleanup when failing to create a workspace",
			func(ctx context.Context) {
				f, err := setupDockerProvider(initialDir+"/bin", "docker")
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(func(cleanupCtx context.Context) {
					_ = f.DevPodProviderDelete(cleanupCtx, "docker")
				})

				initialList, err := f.DevPodList(ctx)
				framework.ExpectNoError(err)
				// Wait for devpod workspace to come online (deadline: 30s)
				err = f.DevPodUp(ctx, "github.com/i/do-not-exist.git")
				framework.ExpectError(err)

				out, err := f.DevPodList(ctx)
				framework.ExpectNoError(err)
				framework.ExpectEqual(out, initialList)
			},
			ginkgo.SpecTimeout(framework.GetTimeout()),
		)

		ginkgo.It(
			"should fail with error when bind mount source does not exist",
			func(ctx context.Context) {
				f, err := setupDockerProvider(initialDir+"/bin", "docker")
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(func(cleanupCtx context.Context) {
					_ = f.DevPodProviderDelete(cleanupCtx, "docker")
				})

				tempDir, err := framework.CopyToTempDir(
					"tests/up/testdata/docker-invalid-bind-mount",
				)
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

				err = f.DevPodUp(ctx, tempDir, "--debug")

				gomega.Expect(err).To(gomega.HaveOccurred())
				gomega.Expect(err.Error()).To(gomega.ContainSubstring("devpod up failed"))
				gomega.Expect(err.Error()).To(gomega.ContainSubstring("exit status 1"))
			},
			ginkgo.SpecTimeout(framework.GetTimeout()),
		)

		ginkgo.It(
			"should fail when CLI mount JSON is malformed",
			func(ctx context.Context) {
				f, err := setupDockerProvider(initialDir+"/bin", "docker")
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(func(cleanupCtx context.Context) {
					_ = f.DevPodProviderDelete(cleanupCtx, "docker")
				})

				tempDir, err := framework.CopyToTempDir("tests/up/testdata/docker")
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)
				ginkgo.DeferCleanup(func(cleanupCtx context.Context) {
					_ = f.DevPodWorkspaceDelete(cleanupCtx, tempDir, "--force")
				})

				_, stderr, err := f.DevPodUpStreams(
					ctx,
					tempDir,
					"--mount",
					`{"type":"bind","source":"/tmp/devpod-cli-mount"`,
				)
				gomega.Expect(err).To(gomega.HaveOccurred())
				gomega.Expect(stderr).
					To(gomega.ContainSubstring(
						`parse --mount JSON`,
					))
			},
			ginkgo.SpecTimeout(framework.GetTimeout()),
		)

		ginkgo.It(
			"should fail when CLI mount JSON target is missing",
			func(ctx context.Context) {
				f, err := setupDockerProvider(initialDir+"/bin", "docker")
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(func(cleanupCtx context.Context) {
					_ = f.DevPodProviderDelete(cleanupCtx, "docker")
				})

				tempDir, err := framework.CopyToTempDir("tests/up/testdata/docker")
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)
				ginkgo.DeferCleanup(func(cleanupCtx context.Context) {
					_ = f.DevPodWorkspaceDelete(cleanupCtx, tempDir, "--force")
				})

				_, stderr, err := f.DevPodUpStreams(
					ctx,
					tempDir,
					"--mount",
					`{"type":"bind","source":"/tmp/devpod-cli-mount"}`,
				)
				gomega.Expect(err).To(gomega.HaveOccurred())
				gomega.Expect(stderr).
					To(gomega.ContainSubstring(
						`target is required`,
					))
			},
			ginkgo.SpecTimeout(framework.GetTimeout()),
		)

		ginkgo.It(
			"should fail when CLI mount JSON bind source is missing",
			func(ctx context.Context) {
				f, err := setupDockerProvider(initialDir+"/bin", "docker")
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(func(cleanupCtx context.Context) {
					_ = f.DevPodProviderDelete(cleanupCtx, "docker")
				})

				tempDir, err := framework.CopyToTempDir("tests/up/testdata/docker")
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)
				ginkgo.DeferCleanup(func(cleanupCtx context.Context) {
					_ = f.DevPodWorkspaceDelete(cleanupCtx, tempDir, "--force")
				})

				_, stderr, err := f.DevPodUpStreams(
					ctx,
					tempDir,
					"--mount",
					`{"type":"bind","target":"/home/vscode/mnt1"}`,
				)
				gomega.Expect(err).To(gomega.HaveOccurred())
				gomega.Expect(stderr).
					To(gomega.ContainSubstring(
						`failed to start dev container: exit status 125`,
					))
			},
			ginkgo.SpecTimeout(framework.GetTimeout()),
		)

		ginkgo.It(
			"should fail when CLI mount JSON type is missing for absolute source",
			func(ctx context.Context) {
				f, err := setupDockerProvider(initialDir+"/bin", "docker")
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(func(cleanupCtx context.Context) {
					_ = f.DevPodProviderDelete(cleanupCtx, "docker")
				})

				tempDir, err := framework.CopyToTempDir("tests/up/testdata/docker-cli-mounts")
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)
				ginkgo.DeferCleanup(func(cleanupCtx context.Context) {
					_ = f.DevPodWorkspaceDelete(cleanupCtx, tempDir, "--force")
				})

				stdout, stderr, err := f.DevPodUpStreams(
					ctx,
					tempDir,
					"--mount",
					`{"source":"${localWorkspaceFolder}/mount1","target":"/home/vscode/mnt1"}`,
				)
				gomega.Expect(err).To(gomega.HaveOccurred())
				gomega.Expect(stdout + stderr).
					To(gomega.ContainSubstring(
						`includes invalid characters for a local volume name`,
					))
			},
			ginkgo.SpecTimeout(framework.GetTimeout()),
		)

		ginkgo.It("ensure workspace cleanup when not a git or folder", func(ctx context.Context) {
			f, err := setupDockerProvider(initialDir+"/bin", "docker")
			framework.ExpectNoError(err)

			initialList, err := f.DevPodList(ctx)
			framework.ExpectNoError(err)
			// Wait for devpod workspace to come online (deadline: 30s)
			err = f.DevPodUp(ctx, "notfound.loft.sh")
			framework.ExpectError(err)

			out, err := f.DevPodList(ctx)
			framework.ExpectNoError(err)
			framework.ExpectEqual(out, initialList)
		}, ginkgo.SpecTimeout(framework.GetTimeout()))
	},
)
