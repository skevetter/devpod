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
		// Documents current behavior: when both image and build are set, the
		// declared image tag is used even with features, which could collide
		// with the upstream registry tag. Changing this would be intentional.
		name:          "preserves build backed services with features",
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
		name:          "preserves build backed services without features",
		composeHelper: &compose.ComposeHelper{Version: "2.30.0"},
		projectName:   "workspace",
		service: &composetypes.ServiceConfig{
			Name:  "app",
			Image: "ghcr.io/example/shared-base:latest",
			Build: &composetypes.BuildConfig{Context: "."},
		},
		hasFeatures: false,
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

type ComposeSuite struct {
	suite.Suite
}

func (s *ComposeSuite) TestStripDigestFromImageRef() {
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
		s.Run(tt.name, func() {
			got := stripDigestFromImageRef(tt.input)
			s.Equal(tt.want, got)
		})
	}
}

func (s *ComposeSuite) TestComposeBuildImageName() {
	for _, tt := range composeBuildImageNameTests {
		s.Run(tt.name, func() {
			got, err := composeBuildImageName(
				tt.composeHelper,
				tt.projectName,
				tt.service,
				tt.hasFeatures,
			)
			s.Require().NoError(err)
			s.Equal(tt.want, got)
		})
	}
}

func (s *ComposeSuite) TestCreateComposeServiceUsesBuildImageName() {
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

	s.Equal("workspace-app:latest", service.Image)
	s.Require().NotNil(service.Build)
	s.Equal("dev_containers_target_stage", service.Build.Target)
	s.Equal("Dockerfile-with-features", service.Build.Dockerfile)
	s.Equal("/tmp/context", service.Build.Context)
	s.Require().NotNil(service.Build.Args)
	s.requireBuildArgValue(service.Build.Args, "FEATURE_FLAG", "true")
	s.requireBuildArgValue(service.Build.Args, "BUILDKIT_INLINE_CACHE", "1")
}

func (s *ComposeSuite) requireBuildArgValue(
	args composetypes.MappingWithEquals,
	key, want string,
) {
	s.T().Helper()

	s.Require().NotNil(args[key])
	s.Equal(want, *args[key])
}

func TestComposeSuite(t *testing.T) {
	suite.Run(t, new(ComposeSuite))
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
