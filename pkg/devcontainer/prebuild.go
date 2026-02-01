package devcontainer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/skevetter/devpod/pkg/devcontainer/build"
	"github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/skevetter/devpod/pkg/driver"
	"github.com/skevetter/devpod/pkg/image"
	"github.com/skevetter/devpod/pkg/provider"
)

func (r *runner) Build(ctx context.Context, options provider.BuildOptions) (string, error) {
	dockerDriver, ok := r.Driver.(driver.DockerDriver)
	if !ok {
		return "", fmt.Errorf("building only supported with docker driver")
	}

	substitutedConfig, substitutionContext, err := r.getSubstitutedConfig(options.CLIOptions)
	if err != nil {
		return "", err
	}

	prebuildRepo := getPrebuildRepository(substitutedConfig)

	// remove build information
	defer func() {
		contextPath := config.GetContextPath(substitutedConfig.Config)
		_ = os.RemoveAll(filepath.Join(contextPath, config.DevPodContextFeatureFolder))
	}()

	// check if we need to build container
	buildInfo, err := r.build(ctx, substitutedConfig, substitutionContext, options)
	if err != nil {
		return "", fmt.Errorf("build image %w", err)
	}

	// have a fallback value for PrebuildHash
	// in some cases it may be empty, and this would lead to
	// invalid reference format during image pushing.
	if buildInfo.PrebuildHash == "" {
		buildInfo.PrebuildHash = "latest"
	}

	// prebuild already exists
	var prebuildImage string
	if options.Repository != "" {
		prebuildImage = options.Repository + ":" + buildInfo.PrebuildHash
	} else if prebuildRepo != "" {
		prebuildImage = prebuildRepo + ":" + buildInfo.PrebuildHash
	} else {
		prebuildImage = build.GetImageName(r.LocalWorkspaceFolder, buildInfo.PrebuildHash)
	}

	if buildInfo.ImageName == prebuildImage {
		return buildInfo.ImageName, nil
	}

	// should we push?
	if options.SkipPush {
		return prebuildImage, nil
	}

	// if push happened during build, skip the separate push workflow
	// When PushDuringBuild is enabled, BuildKit already pushed the image directly to the
	// registry during the build process. Skip the tag/push workflow below to avoid
	// redundant operations. The image is already in the registry.
	if options.PushDuringBuild {
		return prebuildImage, nil
	}

	if isDockerComposeConfig(substitutedConfig.Config) {
		if err := dockerDriver.TagDevContainer(ctx, buildInfo.ImageName, prebuildImage); err != nil {
			return "", fmt.Errorf("tag image %w", err)
		}
	}

	// if no repository is specified, skip push (mirrors devcontainer CLI behavior)
	if options.Repository == "" && prebuildRepo == "" {
		return prebuildImage, nil
	}

	// check if we can push image
	if err := image.CheckPushPermissions(prebuildImage); err != nil {
		return "", fmt.Errorf(
			"cannot push to repository %s. Please make sure you are logged into the registry and credentials are available. (Error: %w)",
			prebuildImage,
			err,
		)
	}

	// Setup all image tags (prebuild and any user defined tags)
	imageRefs := []string{prebuildImage}

	imageRepoName := strings.Split(prebuildImage, ":")
	if buildInfo.Tags != nil {
		for _, tag := range buildInfo.Tags {
			imageRefs = append(imageRefs, imageRepoName[0]+":"+tag)
		}
	}

	// tag the image
	for _, imageRef := range imageRefs {
		if err := dockerDriver.TagDevContainer(ctx, prebuildImage, imageRef); err != nil {
			return "", fmt.Errorf("tag image %w", err)
		}
	}

	// push the image to the registry
	for _, imageRef := range imageRefs {
		if err := dockerDriver.PushDevContainer(ctx, imageRef); err != nil {
			return "", fmt.Errorf("push image %w", err)
		}
	}

	return prebuildImage, nil
}

func getPrebuildRepository(substitutedConfig *config.SubstitutedConfig) string {
	if len(config.GetDevPodCustomizations(substitutedConfig.Config).PrebuildRepository) > 0 {
		return config.GetDevPodCustomizations(substitutedConfig.Config).PrebuildRepository[0]
	}

	return ""
}
