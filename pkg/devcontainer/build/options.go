package build

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/skevetter/devpod/pkg/devcontainer/feature"
	"github.com/skevetter/devpod/pkg/devcontainer/metadata"
	"github.com/skevetter/devpod/pkg/dockerfile"
	"github.com/skevetter/devpod/pkg/id"
	"github.com/skevetter/devpod/pkg/provider"
	"github.com/skevetter/log/hash"
)

type BuildOptions struct {
	BuildArgs map[string]string
	Labels    map[string]string

	CliOpts []string

	Images    []string
	CacheFrom []string
	CacheTo   []string

	Dockerfile string
	Context    string
	Contexts   map[string]string

	Target string

	Load   bool
	Push   bool
	Upload bool
}

func NewOptions(
	dockerfilePath, dockerfileContent string,
	parsedConfig *config.SubstitutedConfig,
	extendedBuildInfo *feature.ExtendedBuildInfo,
	imageName string,
	options provider.BuildOptions,
	prebuildHash string,
) (*BuildOptions, error) {
	var err error

	// extra args?
	buildOptions := &BuildOptions{
		Labels:   map[string]string{},
		Contexts: map[string]string{},
		Load:     true,
	}

	// get build args and target
	buildOptions.BuildArgs, buildOptions.Target = GetBuildArgsAndTarget(parsedConfig, extendedBuildInfo)

	// get cli options
	buildOptions.CliOpts = parsedConfig.Config.GetOptions()

	// get extended build info
	buildOptions.Dockerfile, err = RewriteDockerfile(dockerfileContent, extendedBuildInfo)
	if err != nil {
		return nil, err
	} else if buildOptions.Dockerfile == "" {
		buildOptions.Dockerfile = dockerfilePath
	}

	// add label
	if extendedBuildInfo != nil && extendedBuildInfo.MetadataLabel != "" {
		buildOptions.Labels[metadata.ImageMetadataLabel] = extendedBuildInfo.MetadataLabel
	}

	// other options
	if imageName != "" {
		buildOptions.Images = append(buildOptions.Images, imageName)
	}
	if options.Repository != "" {
		buildOptions.Images = append(buildOptions.Images, options.Repository+":"+prebuildHash)
	}
	for _, prebuildRepository := range options.PrebuildRepositories {
		buildOptions.Images = append(buildOptions.Images, prebuildRepository+":"+prebuildHash)
	}
	buildOptions.Context = config.GetContextPath(parsedConfig.Config)

	// add build arg
	if buildOptions.BuildArgs == nil {
		buildOptions.BuildArgs = map[string]string{}
	}

	// define cache args
	if options.RegistryCache != "" {
		buildOptions.CacheFrom = []string{fmt.Sprintf("type=registry,ref=%s", options.RegistryCache)}
		// only export cache on build not up, otherwise we slow down the workspace start time
		if options.ExportCache {
			buildOptions.CacheTo = []string{fmt.Sprintf("type=registry,ref=%s,mode=max,image-manifest=true", options.RegistryCache)}
		}
	} else {
		buildOptions.BuildArgs["BUILDKIT_INLINE_CACHE"] = "1"
	}

	return buildOptions, nil
}

func GetBuildArgsAndTarget(
	parsedConfig *config.SubstitutedConfig,
	extendedBuildInfo *feature.ExtendedBuildInfo,
) (map[string]string, string) {
	buildArgs := map[string]string{}
	maps.Copy(buildArgs, parsedConfig.Config.GetArgs())

	// get extended build info
	if extendedBuildInfo != nil && extendedBuildInfo.FeaturesBuildInfo != nil {
		featureBuildInfo := extendedBuildInfo.FeaturesBuildInfo

		// track additional build args to include below
		maps.Copy(buildArgs, featureBuildInfo.BuildArgs)
	}

	target := ""
	if extendedBuildInfo != nil && extendedBuildInfo.FeaturesBuildInfo != nil && extendedBuildInfo.FeaturesBuildInfo.OverrideTarget != "" {
		target = extendedBuildInfo.FeaturesBuildInfo.OverrideTarget
	} else if parsedConfig.Config.GetTarget() != "" {
		target = parsedConfig.Config.GetTarget()
	}

	return buildArgs, target
}

func RewriteDockerfile(
	dockerfileContent string,
	extendedBuildInfo *feature.ExtendedBuildInfo,
) (string, error) {
	if extendedBuildInfo != nil && extendedBuildInfo.FeaturesBuildInfo != nil {
		featureBuildInfo := extendedBuildInfo.FeaturesBuildInfo

		// rewrite dockerfile
		finalDockerfileContent := dockerfile.RemoveSyntaxVersion(dockerfileContent)
		finalDockerfileContent = strings.TrimSpace(strings.Join([]string{
			featureBuildInfo.DockerfilePrefixContent,
			strings.TrimSpace(finalDockerfileContent),
			featureBuildInfo.DockerfileContent,
		}, "\n"))

		// write dockerfile with features
		finalDockerfilePath := filepath.Join(featureBuildInfo.FeaturesFolder, "Dockerfile-with-features")
		err := os.WriteFile(finalDockerfilePath, []byte(finalDockerfileContent), 0600)
		if err != nil {
			return "", fmt.Errorf("write Dockerfile with features %w", err)
		}

		return finalDockerfilePath, nil
	}

	return "", nil
}

func GetImageName(localWorkspaceFolder, prebuildHash string) string {
	imageHash := hash.String(localWorkspaceFolder)[:5]
	return id.ToDockerImageName(filepath.Base(localWorkspaceFolder)) + "-" + imageHash + ":" + prebuildHash
}
