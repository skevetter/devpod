package devcontainer

import (
	"os"
	"testing"

	"github.com/skevetter/devpod/pkg/devcontainer/config"
	provider2 "github.com/skevetter/devpod/pkg/provider"
	"github.com/stretchr/testify/suite"
)

type SubstituteTestSuite struct {
	suite.Suite
	runner *runner
}

func TestSubstituteTestSuite(t *testing.T) {
	suite.Run(t, new(SubstituteTestSuite))
}

func (s *SubstituteTestSuite) SetupTest() {
	s.runner = &runner{
		ID:                   "test-id",
		LocalWorkspaceFolder: "/workspace",
		WorkspaceConfig: &provider2.AgentWorkspaceInfo{
			Workspace: &provider2.Workspace{
				ID: "test-workspace",
			},
		},
	}
}

func (s *SubstituteTestSuite) TestSubstitute_WithoutInitEnv() {
	rawConfig := &config.DevContainerConfig{
		ImageContainer: config.ImageContainer{
			Image: "${localEnv:HOME}",
		},
	}
	options := provider2.CLIOptions{}

	result, ctx, err := s.runner.substitute(options, rawConfig)

	s.NoError(err)
	s.NotNil(result)
	s.NotNil(ctx)
	s.Equal(os.Getenv("HOME"), result.Config.Image)
	s.Equal(os.Getenv("HOME"), ctx.Env["HOME"])
}

func (s *SubstituteTestSuite) TestSubstitute_WithInitEnv() {
	rawConfig := &config.DevContainerConfig{
		ImageContainer: config.ImageContainer{
			Image: "${localEnv:CUSTOM_VAR}",
		},
	}
	options := provider2.CLIOptions{
		InitEnv: []string{"CUSTOM_VAR=custom_value"},
	}

	result, ctx, err := s.runner.substitute(options, rawConfig)

	s.NoError(err)
	s.NotNil(result)
	s.NotNil(ctx)
	s.Equal("custom_value", result.Config.Image)
	s.Equal("custom_value", ctx.Env["CUSTOM_VAR"])
}

func (s *SubstituteTestSuite) TestSubstitute_InitEnvOverridesSystemEnv() {
	s.T().Setenv("TEST_VAR", "system_value")

	rawConfig := &config.DevContainerConfig{
		ImageContainer: config.ImageContainer{
			Image: "${localEnv:TEST_VAR}",
		},
	}
	options := provider2.CLIOptions{
		InitEnv: []string{"TEST_VAR=override_value"},
	}

	result, ctx, err := s.runner.substitute(options, rawConfig)

	s.NoError(err)
	s.Equal("override_value", result.Config.Image)
	s.Equal("override_value", ctx.Env["TEST_VAR"])
}

func (s *SubstituteTestSuite) TestSubstitute_MultipleInitEnvVars() {
	rawConfig := &config.DevContainerConfig{
		ImageContainer: config.ImageContainer{
			Image: "${localEnv:REGISTRY}/${localEnv:IMAGE}:${localEnv:TAG}",
		},
	}
	options := provider2.CLIOptions{
		InitEnv: []string{
			"REGISTRY=ghcr.io",
			"IMAGE=myapp",
			"TAG=latest",
		},
	}

	result, ctx, err := s.runner.substitute(options, rawConfig)

	s.NoError(err)
	s.Equal("ghcr.io/myapp:latest", result.Config.Image)
	s.Equal("ghcr.io", ctx.Env["REGISTRY"])
	s.Equal("myapp", ctx.Env["IMAGE"])
	s.Equal("latest", ctx.Env["TAG"])
}

func (s *SubstituteTestSuite) TestSubstitute_EmptyInitEnv() {
	rawConfig := &config.DevContainerConfig{
		ImageContainer: config.ImageContainer{
			Image: "alpine:latest",
		},
	}
	options := provider2.CLIOptions{
		InitEnv: []string{},
	}

	result, _, err := s.runner.substitute(options, rawConfig)

	s.NoError(err)
	s.Equal("alpine:latest", result.Config.Image)
}

func (s *SubstituteTestSuite) TestSubstitute_InitEnvInRemoteEnv() {
	rawConfig := &config.DevContainerConfig{
		ImageContainer: config.ImageContainer{
			Image: "alpine:latest",
		},
		DevContainerConfigBase: config.DevContainerConfigBase{
			RemoteEnv: map[string]string{
				"MY_VAR": "${localEnv:CUSTOM_VAR}",
			},
		},
	}
	options := provider2.CLIOptions{
		InitEnv: []string{"CUSTOM_VAR=test_value"},
	}

	result, ctx, err := s.runner.substitute(options, rawConfig)

	s.NoError(err)
	s.Equal("test_value", result.Config.RemoteEnv["MY_VAR"])
	s.Equal("test_value", ctx.Env["CUSTOM_VAR"])
}

func (s *SubstituteTestSuite) TestSubstitute_MissingVariable() {
	rawConfig := &config.DevContainerConfig{
		ImageContainer: config.ImageContainer{
			Image: "${localEnv:NONEXISTENT}",
		},
	}
	options := provider2.CLIOptions{}

	result, ctx, err := s.runner.substitute(options, rawConfig)

	s.NoError(err)
	s.Equal("", result.Config.Image)
	s.NotContains(ctx.Env, "NONEXISTENT")
}
