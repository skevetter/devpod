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

type NewOptionsParams struct {
	DockerfilePath    string
	DockerfileContent string
	ParsedConfig      *config.SubstitutedConfig
	ExtendedBuildInfo *feature.ExtendedBuildInfo
	ImageName         string
	Options           provider.BuildOptions
	PrebuildHash      string
}

func NewOptions(params NewOptionsParams) (*BuildOptions, error) {
	var err error

	prebuildHash := params.PrebuildHash
	if prebuildHash == "" {
		prebuildHash = "latest"
	}

	// extra args?
	buildOptions := &BuildOptions{
		Labels:   map[string]string{},
		Contexts: map[string]string{},
		Load:     true,
	}

	// get build args and target
	buildOptions.BuildArgs, buildOptions.Target = GetBuildArgsAndTarget(params.ParsedConfig, params.ExtendedBuildInfo)

	// get cli options
	buildOptions.CliOpts = params.ParsedConfig.Config.GetOptions()

	// get extended build info
	buildOptions.Dockerfile, err = RewriteDockerfile(params.DockerfileContent, params.ExtendedBuildInfo)
	if err != nil {
		return nil, err
	} else if buildOptions.Dockerfile == "" {
		buildOptions.Dockerfile = params.DockerfilePath
	}

	// add label
	if params.ExtendedBuildInfo != nil && params.ExtendedBuildInfo.MetadataLabel != "" {
		buildOptions.Labels[metadata.ImageMetadataLabel] = params.ExtendedBuildInfo.MetadataLabel
	}

	// other options
	if params.ImageName != "" {
		buildOptions.Images = append(buildOptions.Images, params.ImageName)
	}
	if params.Options.Repository != "" {
		buildOptions.Images = append(buildOptions.Images, params.Options.Repository+":"+prebuildHash)
	}
	for _, prebuildRepository := range params.Options.PrebuildRepositories {
		buildOptions.Images = append(buildOptions.Images, prebuildRepository+":"+prebuildHash)
	}
	buildOptions.Context = config.GetContextPath(params.ParsedConfig.Config)

	// add build arg
	if buildOptions.BuildArgs == nil {
		buildOptions.BuildArgs = map[string]string{}
	}

	// define cache args
	if params.Options.RegistryCache != "" {
		buildOptions.CacheFrom = []string{fmt.Sprintf("type=registry,ref=%s", params.Options.RegistryCache)}
		// only export cache on build not up, otherwise we slow down the workspace start time
		if params.Options.ExportCache {
			buildOptions.CacheTo = []string{fmt.Sprintf("type=registry,ref=%s,mode=max,image-manifest=true", params.Options.RegistryCache)}
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
