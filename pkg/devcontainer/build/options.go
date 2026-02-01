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

// BuildOptions contains all configuration for building a dev container image.
type BuildOptions struct {
	// BuildArgs are build-time variables passed to the Dockerfile (e.g., VERSION=1.0).
	BuildArgs map[string]string
	// Labels are metadata key-value pairs to attach to the built image.
	Labels map[string]string

	// CliOpts are additional command-line options to pass to the build command.
	CliOpts []string

	// Images are the image names/tags to apply to the built image (e.g., myapp:latest).
	Images []string
	// CacheFrom specifies images to use as cache sources for layer reuse.
	CacheFrom []string
	// CacheTo specifies where to export the build cache (e.g., type=registry,ref=...).
	CacheTo []string

	// Dockerfile is the path to the Dockerfile to build. May be a rewritten Dockerfile with features.
	Dockerfile string
	// Context is the build context path (directory containing files needed for the build).
	Context string
	// Contexts are additional named build contexts (e.g., for COPY --from=name).
	Contexts map[string]string

	// Target specifies the target build stage in a multi-stage Dockerfile.
	Target string

	// Load controls whether to load the built image into the local Docker daemon.
	// When true, uses BuildKit's "moby" exporter which creates a tar and imports it.
	Load bool
	// Push controls whether to push the built image directly to a registry during build.
	// When true, uses BuildKit's "image" exporter which streams directly to the registry.
	// Mutually exclusive with Load.
	Push bool
	// Upload controls whether to upload the build context. Used for remote builds.
	Upload bool
}

// NewOptionsParams contains the parameters needed to create BuildOptions.
type NewOptionsParams struct {
	// DockerfilePath is the path to the original Dockerfile.
	DockerfilePath string
	// DockerfileContent is the content of the Dockerfile, potentially modified with features.
	DockerfileContent string
	// ParsedConfig is the parsed and substituted devcontainer.json configuration.
	ParsedConfig *config.SubstitutedConfig
	// ExtendedBuildInfo contains additional build information including features and metadata.
	ExtendedBuildInfo *feature.ExtendedBuildInfo
	// ImageName is the name to tag the built image with.
	ImageName string
	// Options contains provider-specific build options from the CLI.
	Options provider.BuildOptions
	// PrebuildHash is the hash used for prebuild image tagging.
	PrebuildHash string
}

// NewOptions creates BuildOptions from the provided parameters, configuring the build
// based on the devcontainer.json, features, and CLI options.
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
		// Load controls whether the built image is loaded into the local Docker daemon.
		// When PushDuringBuild is true, this skips loading to avoid the tar export/import overhead.
		Load: !params.Options.PushDuringBuild,
		// Push controls whether BuildKit pushes directly to the registry during build.
		// When true, BuildKit uses the --push flag instead of --load, streaming the image
		// directly to the registry. This is mutually exclusive with Load.
		Push: params.Options.PushDuringBuild,
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
