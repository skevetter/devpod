//go:build linux || darwin || unix

package up

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/skevetter/devpod/e2e/framework"
	"github.com/skevetter/devpod/pkg/compose"
	docker "github.com/skevetter/devpod/pkg/docker"
	"github.com/skevetter/log"
)

var _ = ginkgo.Describe(
	"devpod up docker compose test suite",
	ginkgo.Label("up-docker-compose", "build"),
	func() {
		var f *framework.Framework
		var dockerHelper *docker.DockerHelper
		var composeHelper *compose.ComposeHelper
		var initialDir string

		ginkgo.BeforeEach(func(ctx context.Context) {
			var err error
			initialDir, err = os.Getwd()
			framework.ExpectNoError(err)

			dockerHelper = &docker.DockerHelper{DockerCommand: "docker", Log: log.Default}
			composeHelper, err = compose.NewComposeHelper(dockerHelper)
			framework.ExpectNoError(err)

			f, err = setupDockerProvider(initialDir+"/bin", "docker")
			framework.ExpectNoError(err)
		})

		ginkgo.It("should start a new workspace with multistage build", func(ctx context.Context) {
			tempDir, err := setupWorkspace(
				"tests/up-docker-compose/testdata/docker-compose-with-multi-stage-build",
				initialDir,
				f,
			)
			framework.ExpectNoError(err)

			// Wait for devpod workspace to come online (deadline: 30s)
			err = f.DevPodUp(ctx, tempDir, "--debug")
			framework.ExpectNoError(err)
		}, ginkgo.SpecTimeout(framework.GetTimeout()*3))

		ginkgo.It("should NOT delete container when rebuild fails", func(ctx context.Context) {
			tempDir, err := setupWorkspace(
				"tests/up-docker-compose/testdata/docker-compose-rebuild-fail",
				initialDir,
				f,
			)
			framework.ExpectNoError(err)

			ginkgo.By("Starting DevPod")
			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			workspace, err := f.FindWorkspace(ctx, tempDir)
			framework.ExpectNoError(err)

			ginkgo.By("Should start a docker-compose container")
			var ids []string
			gomega.Eventually(func() int {
				ids, err = dockerHelper.FindContainer(ctx, []string{
					fmt.Sprintf(
						"%s=%s",
						compose.ProjectLabel,
						composeHelper.GetProjectName(workspace.UID),
					),
					fmt.Sprintf("%s=%s", compose.ServiceLabel, "app"),
				})
				if err != nil {
					return 0
				}
				return len(ids)
			}).
				WithTimeout(60*time.Second).
				WithPolling(1*time.Second).
				Should(gomega.Equal(1), "1 compose container to be created")

			ginkgo.By("Modifying .devcontainer.json with failing changes")
			origPath := filepath.Join(tempDir, ".devcontainer.json")
			err = os.Chown(origPath, os.Getuid(), os.Getgid())
			framework.ExpectNoError(err)
			// #nosec G302 -- TODO Consider using a more secure permission setting and ownership if needed.
			err = os.Chmod(origPath, 0o666)
			framework.ExpectNoError(err)
			err = os.Remove(origPath)
			framework.ExpectNoError(err)

			failingConfig, err := os.Open(filepath.Join(tempDir, "fail.devcontainer.json"))
			framework.ExpectNoError(err)
			defer func() { _ = failingConfig.Close() }()

			newConfig, err := os.Create(origPath)
			framework.ExpectNoError(err)
			defer func() { _ = newConfig.Close() }()

			_, err = io.Copy(newConfig, failingConfig)
			framework.ExpectNoError(err)

			ginkgo.By("Starting DevPod again with --recreate")
			err = f.DevPodUp(ctx, tempDir, "--debug", "--recreate")
			framework.ExpectError(err)

			ginkgo.By("Should leave original container running")
			ids2, err := dockerHelper.FindContainer(ctx, []string{
				fmt.Sprintf(
					"%s=%s",
					compose.ProjectLabel,
					composeHelper.GetProjectName(workspace.UID),
				),
				fmt.Sprintf("%s=%s", compose.ServiceLabel, "app"),
			})
			framework.ExpectNoError(err)
			gomega.Expect(ids2[0]).To(gomega.Equal(ids[0]), "Should use original container")
		})

		ginkgo.It(
			"should rebuild features from an extra devcontainer file",
			func(ctx context.Context) {
				tempDir, err := setupWorkspace(
					"tests/up-docker-compose/testdata/docker-compose-rebuild-success",
					initialDir,
					f,
				)
				framework.ExpectNoError(err)
				featureDir := filepath.Join(tempDir, "feature")
				framework.ExpectNoError(os.Mkdir(featureDir, 0o750))
				framework.ExpectNoError(
					os.WriteFile(filepath.Join(featureDir, "devcontainer-feature.json"), []byte(`{
				"id": "marker", "version": "1.0.0", "name": "Marker",
				"options": {"value": {"type": "string", "default": "default"}}
			}`), 0o600),
				)
				framework.ExpectNoError(
					os.WriteFile(filepath.Join(featureDir, "install.sh"), []byte(
						"#!/bin/sh\nset -eu\nprintf '%s' \"$VALUE\" > /usr/local/share/extra-feature-marker\n",
					), 0o600),
				)
				extraPath := filepath.Join(ginkgo.GinkgoT().TempDir(), "extra.json")
				framework.ExpectNoError(
					os.WriteFile(
						extraPath,
						[]byte(`{"remoteEnv": {"EXTRA_RUNTIME": "kept"}}`),
						0o600,
					),
				)
				framework.ExpectNoError(
					f.DevPodUp(ctx, tempDir, "--extra-devcontainer-path", extraPath),
				)

				ginkgo.By("Leaving a running container unchanged without --recreate")
				framework.ExpectNoError(os.WriteFile(extraPath, []byte(
					`{"features": {"./feature": {"value": "first"}}, "remoteEnv": {"EXTRA_RUNTIME": "kept"}}`,
				), 0o600))
				framework.ExpectNoError(
					f.DevPodUp(ctx, tempDir, "--extra-devcontainer-path", extraPath),
				)
				_, err = f.DevPodSSH(
					ctx,
					tempDir,
					"test ! -e /usr/local/share/extra-feature-marker",
				)
				framework.ExpectNoError(err)

				ginkgo.By("Installing extra features and applying changed options on rebuild")
				for _, value := range []string{"first", "second"} {
					framework.ExpectNoError(os.WriteFile(extraPath, fmt.Appendf(
						nil,
						`{"features": {"./feature": {"value": %q}}, "remoteEnv": {"EXTRA_RUNTIME": "kept"}}`,
						value,
					), 0o600))
					framework.ExpectNoError(
						f.DevPodUp(
							ctx,
							tempDir,
							"--extra-devcontainer-path",
							extraPath,
							"--recreate",
						),
					)
					output, err := f.DevPodSSH(
						ctx,
						tempDir,
						`test "$EXTRA_RUNTIME" = kept && cat /usr/local/share/extra-feature-marker`,
					)
					framework.ExpectNoError(err)
					gomega.Expect(strings.TrimSpace(output)).To(gomega.Equal(value))
				}
			},
			ginkgo.SpecTimeout(framework.GetTimeout()*3),
		)

		ginkgo.It("should delete container upon successful rebuild", func(ctx context.Context) {
			tempDir, err := setupWorkspace(
				"tests/up-docker-compose/testdata/docker-compose-rebuild-success",
				initialDir,
				f,
			)
			framework.ExpectNoError(err)

			ginkgo.By("Starting DevPod")
			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			workspace, err := f.FindWorkspace(ctx, tempDir)
			framework.ExpectNoError(err)

			ginkgo.By("Should start a docker-compose container")
			var ids []string
			gomega.Eventually(func() int {
				ids, err = dockerHelper.FindContainer(ctx, []string{
					fmt.Sprintf(
						"%s=%s",
						compose.ProjectLabel,
						composeHelper.GetProjectName(workspace.UID),
					),
					fmt.Sprintf("%s=%s", compose.ServiceLabel, "app"),
				})
				if err != nil {
					return 0
				}
				return len(ids)
			}).
				WithTimeout(60*time.Second).
				WithPolling(1*time.Second).
				Should(gomega.Equal(1), "1 compose container to be created")

			ginkgo.By("Starting DevPod again with --recreate")
			err = f.DevPodUp(ctx, tempDir, "--debug", "--recreate")
			framework.ExpectNoError(err)

			ginkgo.By("Should start a new docker-compose container on rebuild")
			ids2, err := dockerHelper.FindContainer(ctx, []string{
				fmt.Sprintf(
					"%s=%s",
					compose.ProjectLabel,
					composeHelper.GetProjectName(workspace.UID),
				),
				fmt.Sprintf("%s=%s", compose.ServiceLabel, "app"),
			})
			framework.ExpectNoError(err)
			gomega.Expect(ids2[0]).NotTo(gomega.Equal(ids[0]), "Should restart container")
		})
	},
)
