package build

import (
	"context"
	"os"
	"path/filepath"
	"runtime"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
	"github.com/skevetter/devpod/pkg/devcontainer/build"
	"github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/skevetter/devpod/pkg/docker"
	"github.com/skevetter/devpod/pkg/dockerfile"
	"github.com/skevetter/log"
)

const (
	prebuildRepoName = "test-repo"
	osWindows        = "windows"
)

func prepareDockerfileContent(dockerfilePath string) (string, error) {
	dockerfileContent, err := os.ReadFile(dockerfilePath) // #nosec G304 -- test file path
	if err != nil {
		return "", err
	}
	_, modifiedDockerfileContents, err := dockerfile.EnsureFinalStageName(
		string(dockerfileContent),
		config.DockerfileDefaultTarget,
	)
	if err != nil {
		return "", err
	}
	contentToParse := modifiedDockerfileContents
	if contentToParse == "" {
		contentToParse = string(dockerfileContent)
	}
	return contentToParse, nil
}

func getDevcontainerConfig(dir string) *config.DevContainerConfig {
	return &config.DevContainerConfig{
		DevContainerConfigBase: config.DevContainerConfigBase{
			Name: "Build Example",
		},
		DevContainerActions: config.DevContainerActions{},
		NonComposeBase:      config.NonComposeBase{},
		ImageContainer:      config.ImageContainer{},
		ComposeContainer:    config.ComposeContainer{},
		DockerfileContainer: config.DockerfileContainer{
			Build: &config.ConfigBuildOptions{
				Dockerfile: "Dockerfile",
				Context:    ".",
				Options:    []string{"--label=test=VALUE"},
			},
		},
		Origin: dir + "/.devcontainer/devcontainer.json",
	}
}

var _ = ginkgo.Describe("devpod build test suite", ginkgo.Label("build"), ginkgo.Ordered, func() {
	var initialDir string
	var dockerHelper *docker.DockerHelper

	ginkgo.BeforeEach(func() {
		var err error
		initialDir, err = os.Getwd()
		framework.ExpectNoError(err)
		dockerHelper = &docker.DockerHelper{DockerCommand: "docker", Log: log.Default}
	})

	ginkgo.It("build docker buildx",
		ginkgo.SpecTimeout(framework.GetTimeout()),
		func(ctx context.Context) {
			f := framework.NewDefaultFramework(initialDir + "/bin")
			tempDir, err := framework.CopyToTempDir("tests/build/testdata/docker")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			_ = f.DevPodProviderDelete(ctx, "docker")
			err = f.DevPodProviderAdd(ctx, "docker")
			framework.ExpectNoError(err)
			err = f.DevPodProviderUse(ctx, "docker")
			framework.ExpectNoError(err)

			cfg := getDevcontainerConfig(tempDir)

			dockerfilePath := tempDir + "/.devcontainer/Dockerfile"
			contentToParse, err := prepareDockerfileContent(dockerfilePath)
			framework.ExpectNoError(err)

			// do the build
			platforms := "linux/amd64,linux/arm64"
			err = f.DevPodBuild(
				ctx,
				tempDir,
				"--force-build",
				"--platform",
				platforms,
				"--repository",
				prebuildRepoName,
				"--skip-push",
			)
			framework.ExpectNoError(err)

			// parse the dockerfile
			file, err := dockerfile.Parse(contentToParse)
			framework.ExpectNoError(err)
			info := &config.ImageBuildInfo{Dockerfile: file}

			// make sure images are there
			prebuildHash, err := config.CalculatePrebuildHash(config.PrebuildHashParams{
				Config:            cfg,
				Platform:          "linux/amd64",
				Architecture:      "amd64",
				ContextPath:       filepath.Dir(cfg.Origin),
				DockerfilePath:    dockerfilePath,
				DockerfileContent: contentToParse,
				BuildInfo:         info,
				Log:               log.Default,
			})
			framework.ExpectNoError(err)
			_, err = dockerHelper.InspectImage(ctx, prebuildRepoName+":"+prebuildHash, false)
			framework.ExpectNoError(err)

			prebuildHash, err = config.CalculatePrebuildHash(config.PrebuildHashParams{
				Config:            cfg,
				Platform:          "linux/arm64",
				Architecture:      "arm64",
				ContextPath:       filepath.Dir(cfg.Origin),
				DockerfilePath:    dockerfilePath,
				DockerfileContent: contentToParse,
				BuildInfo:         info,
				Log:               log.Default,
			})
			framework.ExpectNoError(err)

			details, err := dockerHelper.InspectImage(ctx, prebuildRepoName+":"+prebuildHash, false)
			framework.ExpectNoError(err)
			framework.ExpectEqual(
				details.Config.Labels["test"],
				"VALUE",
				"should contain test label",
			)
		})

	ginkgo.It(
		"should build image without repository specified if skip-push flag is set",
		ginkgo.SpecTimeout(framework.GetTimeout()),
		func(ctx context.Context) {
			f := framework.NewDefaultFramework(initialDir + "/bin")
			tempDir, err := framework.CopyToTempDir("tests/build/testdata/docker")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			_ = f.DevPodProviderDelete(ctx, "docker")
			err = f.DevPodProviderAdd(ctx, "docker")
			framework.ExpectNoError(err)
			err = f.DevPodProviderUse(ctx, "docker")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, tempDir)

			cfg := getDevcontainerConfig(tempDir)

			dockerfilePath := tempDir + "/.devcontainer/Dockerfile"
			contentToParse, err := prepareDockerfileContent(dockerfilePath)
			framework.ExpectNoError(err)

			// do the build
			err = f.DevPodBuild(ctx, tempDir, "--skip-push")
			framework.ExpectNoError(err)

			// parse the dockerfile
			file, err := dockerfile.Parse(contentToParse)
			framework.ExpectNoError(err)
			info := &config.ImageBuildInfo{Dockerfile: file}

			// make sure images are there
			prebuildHash, err := config.CalculatePrebuildHash(config.PrebuildHashParams{
				Config:            cfg,
				Platform:          "linux/" + runtime.GOARCH,
				Architecture:      runtime.GOARCH,
				ContextPath:       filepath.Dir(cfg.Origin),
				DockerfilePath:    dockerfilePath,
				DockerfileContent: contentToParse,
				BuildInfo:         info,
				Log:               log.Default,
			})
			framework.ExpectNoError(err)
			_, err = dockerHelper.InspectImage(
				ctx,
				build.GetImageName(tempDir, prebuildHash),
				false,
			)
			framework.ExpectNoError(err)
		},
	)

	ginkgo.It(
		"should build the image of the referenced service from the docker compose file",
		ginkgo.SpecTimeout(framework.GetTimeout()),
		func(ctx context.Context) {
			f := framework.NewDefaultFramework(initialDir + "/bin")
			tempDir, err := framework.CopyToTempDir("tests/build/testdata/docker-compose")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			_ = f.DevPodProviderDelete(ctx, "docker")
			err = f.DevPodProviderAdd(ctx, "docker")
			framework.ExpectNoError(err)
			err = f.DevPodProviderUse(ctx, "docker")
			framework.ExpectNoError(err)

			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, tempDir)

			prebuildRepo := prebuildRepoName

			// do the build
			err = f.DevPodBuild(ctx, tempDir, "--repository", prebuildRepo, "--skip-push")
			framework.ExpectNoError(err)
		},
	)

	ginkgo.It(
		"should build docker-compose with features when build context differs from devcontainer location",
		ginkgo.SpecTimeout(framework.GetTimeout()),
		func(ctx context.Context) {
			f := framework.NewDefaultFramework(initialDir + "/bin")
			tempDir, err := framework.CopyToTempDir(
				"tests/build/testdata/docker-compose-features-context",
			)
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			_ = f.DevPodProviderDelete(ctx, "docker")
			err = f.DevPodProviderAdd(ctx, "docker")
			framework.ExpectNoError(err)
			err = f.DevPodProviderUse(ctx, "docker")
			framework.ExpectNoError(err)

			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, tempDir)

			err = f.DevPodBuild(ctx, tempDir, "--skip-push")
			framework.ExpectNoError(err)
		},
	)

	ginkgo.It("build docker internal buildkit",
		ginkgo.SpecTimeout(framework.GetTimeout()),
		func(ctx context.Context) {
			f := framework.NewDefaultFramework(initialDir + "/bin")
			tempDir, err := framework.CopyToTempDir("tests/build/testdata/docker")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			_ = f.DevPodProviderDelete(ctx, "docker")
			err = f.DevPodProviderAdd(ctx, "docker")
			framework.ExpectNoError(err)
			err = f.DevPodProviderUse(ctx, "docker")
			framework.ExpectNoError(err)

			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, tempDir)

			cfg := getDevcontainerConfig(tempDir)

			dockerfilePath := tempDir + "/.devcontainer/Dockerfile"
			contentToParse, err := prepareDockerfileContent(dockerfilePath)
			framework.ExpectNoError(err)

			prebuildRepo := prebuildRepoName

			// do the build
			err = f.DevPodBuild(
				ctx,
				tempDir,
				"--force-build",
				"--force-internal-buildkit",
				"--repository",
				prebuildRepo,
				"--skip-push",
			)
			framework.ExpectNoError(err)

			// parse the dockerfile
			file, err := dockerfile.Parse(contentToParse)
			framework.ExpectNoError(err)
			info := &config.ImageBuildInfo{Dockerfile: file}

			// make sure images are there
			prebuildHash, err := config.CalculatePrebuildHash(config.PrebuildHashParams{
				Config:            cfg,
				Platform:          "linux/" + runtime.GOARCH,
				Architecture:      runtime.GOARCH,
				ContextPath:       filepath.Dir(cfg.Origin),
				DockerfilePath:    dockerfilePath,
				DockerfileContent: contentToParse,
				BuildInfo:         info,
				Log:               log.Default,
			})
			framework.ExpectNoError(err)

			_, err = dockerHelper.InspectImage(ctx, prebuildRepo+":"+prebuildHash, false)
			framework.ExpectNoError(err)
		})

	ginkgo.It("build kubernetes dockerless",
		ginkgo.SpecTimeout(framework.GetTimeout()),
		func(ctx context.Context) {
			if runtime.GOOS == osWindows {
				ginkgo.Skip("skipping on windows")
			}

			f := framework.NewDefaultFramework(initialDir + "/bin")
			tempDir, err := framework.CopyToTempDir("tests/build/testdata/kubernetes")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			_ = f.DevPodProviderDelete(ctx, "kubernetes")
			err = f.DevPodProviderAdd(ctx, "kubernetes")
			framework.ExpectNoError(err)
			err = f.DevPodProviderUse(
				ctx,
				"kubernetes",
				"-o",
				"KUBERNETES_NAMESPACE=devpod",
			)
			framework.ExpectNoError(err)

			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, tempDir)

			// do the up
			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			// check if ssh works
			out, err := f.DevPodSSH(ctx, tempDir, "echo -n $MY_TEST")
			framework.ExpectNoError(err)
			framework.ExpectEqual(out, "test456", "should contain my-test")
		})

	ginkgo.It("rebuild kubernetes dockerless",
		ginkgo.SpecTimeout(framework.GetTimeout()),
		func(ctx context.Context) {
			validateKubernetesDeploymentWithoutDocker(
				ctx,
				initialDir,
				func(ctx context.Context, f *framework.Framework, tempDir string) error {
					return f.DevPodUpRecreate(ctx, tempDir)
				},
			)
		})

	ginkgo.It("reset kubernetes dockerless",
		ginkgo.SpecTimeout(framework.GetTimeout()),
		func(ctx context.Context) {
			validateKubernetesDeploymentWithoutDocker(
				ctx,
				initialDir,
				func(ctx context.Context, f *framework.Framework, tempDir string) error {
					return f.DevPodUpReset(ctx, tempDir)
				},
			)
		})
})

func validateKubernetesDeploymentWithoutDocker(
	ctx context.Context,
	initialDir string,
	action func(context.Context, *framework.Framework, string) error,
) {
	if runtime.GOOS == osWindows {
		ginkgo.Skip("skipping on Windows")
	}

	f := framework.NewDefaultFramework(initialDir + "/bin")
	tempDir, err := framework.CopyToTempDir("tests/build/testdata/kubernetes")
	framework.ExpectNoError(err)
	ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

	_ = f.DevPodProviderDelete(ctx, "kubernetes")
	err = f.DevPodProviderAdd(ctx, "kubernetes")
	framework.ExpectNoError(err)
	err = f.DevPodProviderUse(
		ctx,
		"kubernetes",
		"-o",
		"KUBERNETES_NAMESPACE=devpod",
	)
	framework.ExpectNoError(err)

	ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, tempDir)

	err = f.DevPodUp(ctx, tempDir)
	framework.ExpectNoError(err)

	_, err = f.DevPodSSH(ctx, tempDir, "touch /workspaces/"+filepath.Base(tempDir)+"/DATA")
	framework.ExpectNoError(err)
	_, err = f.DevPodSSH(ctx, tempDir, "touch /ROOTFS")
	framework.ExpectNoError(err)

	err = action(ctx, f, tempDir)
	framework.ExpectNoError(err)

	_, err = f.DevPodSSH(ctx, tempDir, "ls /workspaces/"+filepath.Base(tempDir)+"/DATA")
	framework.ExpectNoError(err)
	_, err = f.DevPodSSH(ctx, tempDir, "ls /ROOTFS")
	framework.ExpectError(err)
}
