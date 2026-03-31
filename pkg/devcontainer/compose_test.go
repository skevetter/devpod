package devcontainer

import (
	"path/filepath"
	"testing"

	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/pkg/compose"
	"github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/skevetter/devpod/pkg/devcontainer/feature"
	logLib "github.com/skevetter/log"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type composeBuildImageNameTestCase struct {
	name          string
	composeHelper *compose.ComposeHelper
	projectName   string
	service       *composetypes.ServiceConfig
	hasFeatures   bool
	want          string
}

var composeBuildImageNameTests = []composeBuildImageNameTestCase{
	{
		name:          "keeps original image without features",
		composeHelper: &compose.ComposeHelper{Version: "2.30.0"},
		projectName:   "workspace",
		service: &composetypes.ServiceConfig{
			Name:  "app",
			Image: "ghcr.io/example/shared-base:latest",
		},
		want: "ghcr.io/example/shared-base:latest",
	},
	{
		name:          "uses workspace image for image backed features",
		composeHelper: &compose.ComposeHelper{Version: "2.30.0"},
		projectName:   "workspace",
		service: &composetypes.ServiceConfig{
			Name:  "app",
			Image: "ghcr.io/example/shared-base:latest",
		},
		hasFeatures: true,
		want:        "workspace-app",
	},
	{
		name:          "uses compose version separator for image backed features",
		composeHelper: &compose.ComposeHelper{Version: "2.7.0"},
		projectName:   "workspace",
		service: &composetypes.ServiceConfig{
			Name:  "app",
			Image: "ghcr.io/example/shared-base:latest",
		},
		hasFeatures: true,
		want:        "workspace_app",
	},
	{
		name:          "preserves build backed services",
		composeHelper: &compose.ComposeHelper{Version: "2.30.0"},
		projectName:   "workspace",
		service: &composetypes.ServiceConfig{
			Name:  "app",
			Image: "ghcr.io/example/shared-base:latest",
			Build: &composetypes.BuildConfig{Context: "."},
		},
		hasFeatures: true,
		want:        "ghcr.io/example/shared-base:latest",
	},
	{
		name:          "uses default image when compose image is empty",
		composeHelper: &compose.ComposeHelper{Version: "2.30.0"},
		projectName:   "workspace",
		service: &composetypes.ServiceConfig{
			Name: "app",
		},
		hasFeatures: true,
		want:        "workspace-app",
	},
}

func TestStripDigestFromImageRef(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "digest reference",
			input: "registry.example.com/app:1.2.3@sha256:abcdef",
			want:  "registry.example.com/app:1.2.3",
		},
		{
			name:  "no digest",
			input: "registry.example.com/app:1.2.3",
			want:  "registry.example.com/app:1.2.3",
		},
		{
			name:  "digest without tag",
			input: "registry.example.com/app@sha256:abcdef",
			want:  "registry.example.com/app",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := stripDigestFromImageRef(tt.input)
			if got != tt.want {
				t.Fatalf("stripDigestFromImageRef(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestComposeBuildImageName(t *testing.T) {
	t.Parallel()

	for _, tt := range composeBuildImageNameTests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := composeBuildImageName(
				tt.composeHelper,
				tt.projectName,
				tt.service,
				tt.hasFeatures,
			)
			if err != nil {
				t.Fatalf("composeBuildImageName() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("composeBuildImageName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCreateComposeServiceUsesBuildImageName(t *testing.T) {
	t.Parallel()

	r := &runner{}
	service := r.createComposeService(
		&composetypes.ServiceConfig{
			Name:  "app",
			Image: "ghcr.io/example/shared-base:latest",
			Build: &composetypes.BuildConfig{Target: "original-target"},
		},
		"workspace-app:latest",
		"Dockerfile-with-features",
		"/tmp/context",
		&feature.BuildInfo{
			OverrideTarget: "dev_containers_target_stage",
			BuildArgs: map[string]string{
				"FEATURE_FLAG": "true",
			},
		},
	)

	require.Equal(t, "workspace-app:latest", service.Image)
	require.NotNil(t, service.Build)
	require.Equal(t, "dev_containers_target_stage", service.Build.Target)
	require.Equal(t, "Dockerfile-with-features", service.Build.Dockerfile)
	require.Equal(t, "/tmp/context", service.Build.Context)
	require.NotNil(t, service.Build.Args)
	requireBuildArgValue(t, service.Build.Args, "FEATURE_FLAG", "true")
	requireBuildArgValue(t, service.Build.Args, "BUILDKIT_INLINE_CACHE", "1")
}

func requireBuildArgValue(
	t *testing.T,
	args composetypes.MappingWithEquals,
	key, want string,
) {
	t.Helper()

	require.NotNil(t, args[key])
	require.Equal(t, want, *args[key])
}

type PrepareBuildContextSuite struct {
	suite.Suite
	runner *runner
}

func (s *PrepareBuildContextSuite) SetupTest() {
	s.runner = &runner{Log: logLib.NewDiscardLogger(logrus.InfoLevel)}
}

func (s *PrepareBuildContextSuite) TestNoContextRelativePath() {
	result, err := s.runner.prepareBuildContext(
		&composetypes.ServiceConfig{Name: "test-service"},
		"/tmp/features/Dockerfile",
		"FROM alpine",
		&feature.BuildInfo{FeaturesFolder: "/tmp/features/folder"},
	)

	s.NoError(err)
	s.False(
		filepath.IsAbs(result.dockerfilePathInContext),
		"dockerfilePathInContext should be relative",
	)
	s.Equal("Dockerfile", result.dockerfilePathInContext)
	s.Equal("/tmp/features", result.context)
}

func (s *PrepareBuildContextSuite) TestNilBuildRelativePath() {
	result, err := s.runner.prepareBuildContext(
		&composetypes.ServiceConfig{Name: "test-service", Build: nil},
		"/workspace/.devcontainer/features/Dockerfile",
		"FROM alpine",
		&feature.BuildInfo{FeaturesFolder: "/workspace/.devcontainer/features/folder"},
	)

	s.NoError(err)
	s.False(
		filepath.IsAbs(result.dockerfilePathInContext),
		"dockerfilePathInContext should be relative",
	)
	s.Equal("Dockerfile", result.dockerfilePathInContext)
	s.Equal("/workspace/.devcontainer/features", result.context)
}

func (s *PrepareBuildContextSuite) TestCustomBuildContext() {
	dockerfileContent := "FROM alpine\nCOPY ./" + config.DevPodContextFeatureFolder + "/ /tmp/build-features/"

	result, err := s.runner.prepareBuildContext(
		&composetypes.ServiceConfig{
			Name: "test-service",
			Build: &composetypes.BuildConfig{
				Context: "/workspace",
			},
		},
		"/workspace/.devcontainer/features/Dockerfile",
		dockerfileContent,
		&feature.BuildInfo{FeaturesFolder: "/workspace/.devcontainer/features/folder"},
	)

	s.NoError(err)
	s.False(
		filepath.IsAbs(result.dockerfilePathInContext),
		"dockerfilePathInContext should be relative",
	)
	s.Equal(".devcontainer/features/Dockerfile", result.dockerfilePathInContext)
	s.Equal("/workspace", result.context)
	s.Contains(result.dockerfileContent, "COPY ./.devcontainer/features/folder/")
	s.NotContains(result.dockerfileContent, "COPY ./"+config.DevPodContextFeatureFolder+"/")
}

func (s *PrepareBuildContextSuite) TestCustomBuildContextPreservesWhitespace() {
	dockerfileContent := "COPY  ./" + config.DevPodContextFeatureFolder + "/ /tmp/\n" +
		"ADD\t./" + config.DevPodContextFeatureFolder + "/ /other/"

	result, err := s.runner.prepareBuildContext(
		&composetypes.ServiceConfig{
			Name:  "test-service",
			Build: &composetypes.BuildConfig{Context: "/workspace"},
		},
		"/workspace/.devcontainer/features/Dockerfile",
		dockerfileContent,
		&feature.BuildInfo{FeaturesFolder: "/workspace/.devcontainer/features/folder"},
	)

	s.NoError(err)
	s.Contains(result.dockerfileContent, "COPY  ./.devcontainer/features/folder/")
	s.Contains(result.dockerfileContent, "ADD\t./.devcontainer/features/folder/")
}

func (s *PrepareBuildContextSuite) TestCustomBuildContextNoReplacementNeeded() {
	dockerfileContent := "FROM alpine\nRUN echo hello"

	result, err := s.runner.prepareBuildContext(
		&composetypes.ServiceConfig{
			Name:  "test-service",
			Build: &composetypes.BuildConfig{Context: "/workspace"},
		},
		"/workspace/.devcontainer/features/Dockerfile",
		dockerfileContent,
		&feature.BuildInfo{FeaturesFolder: "/workspace/.devcontainer/features/folder"},
	)

	s.NoError(err)
	s.Equal(dockerfileContent, result.dockerfileContent, "content should be unchanged")
}

func (s *PrepareBuildContextSuite) TestCustomBuildContextEmptyContext() {
	result, err := s.runner.prepareBuildContext(
		&composetypes.ServiceConfig{
			Name:  "test-service",
			Build: &composetypes.BuildConfig{Context: ""},
		},
		"/workspace/.devcontainer/features/Dockerfile",
		"FROM alpine",
		&feature.BuildInfo{FeaturesFolder: "/workspace/.devcontainer/features/folder"},
	)

	s.NoError(err)
	s.Equal("Dockerfile", result.dockerfilePathInContext)
	s.Equal("/workspace/.devcontainer/features", result.context)
}

func TestPrepareBuildContextSuite(t *testing.T) {
	suite.Run(t, new(PrepareBuildContextSuite))
}
