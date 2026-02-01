package driver

import (
	"context"
	"io"

	"github.com/skevetter/devpod/pkg/compose"
	config2 "github.com/skevetter/devpod/pkg/config"
	"github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/skevetter/devpod/pkg/devcontainer/feature"
	"github.com/skevetter/devpod/pkg/docker"
	"github.com/skevetter/devpod/pkg/provider"
)

type RunDockerDevContainerParams struct {
	WorkspaceID  string
	Options      *RunOptions
	ParsedConfig *config.DevContainerConfig
	IDE          string
	IDEOptions   map[string]config2.OptionValue
}

type BuildRequest struct {
	PrebuildHash         string
	ParsedConfig         *config.SubstitutedConfig
	ExtendedBuildInfo    *feature.ExtendedBuildInfo
	DockerfilePath       string
	DockerfileContent    string
	LocalWorkspaceFolder string
	Options              provider.BuildOptions
}

type DockerDriver interface {
	Driver

	// InspectImage inspects the given image name
	InspectImage(ctx context.Context, imageName string) (*config.ImageDetails, error)

	// GetImageTag returns latest tag for input image id
	GetImageTag(ctx context.Context, imageName string) (string, error)

	// RunDockerDevContainer runs a docker devcontainer
	RunDockerDevContainer(ctx context.Context, params *RunDockerDevContainerParams) error

	// BuildDevContainer builds a devcontainer
	BuildDevContainer(ctx context.Context, req BuildRequest) (*config.BuildInfo, error)

	// PushDevContainer pushes the given image to a registry
	PushDevContainer(ctx context.Context, image string) error

	// TagDevContainer tags the given image with the given tag
	TagDevContainer(ctx context.Context, image, tag string) error

	// UpdateContainerUserUID updates the container user UID/GID to match local user
	UpdateContainerUserUID(ctx context.Context, workspaceId string, parsedConfig *config.DevContainerConfig, writer io.Writer) error

	// ComposeHelper returns the compose helper
	ComposeHelper() (*compose.ComposeHelper, error)

	// DockerHellper returns the docker helper
	DockerHelper() (*docker.DockerHelper, error)
}
