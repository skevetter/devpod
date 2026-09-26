package devcontainer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/skevetter/devpod/pkg/devcontainer/config"
	provider2 "github.com/skevetter/devpod/pkg/provider"
	"github.com/skevetter/log"
	"github.com/stretchr/testify/suite"
)

const (
	testContainerWorkspaceFolder = "/workspaces/test-workspace"
	testVolumeType               = "volume"
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
		Log:                  log.Discard,
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

func (s *SubstituteTestSuite) TestSubstitute_AdditionalFeatures() {
	rawConfig := &config.DevContainerConfig{
		ImageContainer: config.ImageContainer{Image: "alpine:latest"},
	}
	options := provider2.CLIOptions{
		AdditionalFeatures: `{"ghcr.io/devcontainers/features/git:1": {}}`,
	}

	result, _, err := s.runner.substitute(options, rawConfig)

	s.NoError(err)
	s.Contains(result.Config.Features, "ghcr.io/devcontainers/features/git:1")
}

func (s *SubstituteTestSuite) TestSubstitute_AdditionalFeaturesMergesWithExisting() {
	rawConfig := &config.DevContainerConfig{
		ImageContainer: config.ImageContainer{Image: "alpine:latest"},
		DevContainerConfigBase: config.DevContainerConfigBase{
			Features: map[string]any{
				"ghcr.io/devcontainers/features/node:1": map[string]any{"version": "20"},
			},
		},
	}
	options := provider2.CLIOptions{
		AdditionalFeatures: `{"ghcr.io/devcontainers/features/git:1": {}}`,
	}

	result, _, err := s.runner.substitute(options, rawConfig)

	s.NoError(err)
	s.Len(result.Config.Features, 2)
	s.Contains(result.Config.Features, "ghcr.io/devcontainers/features/node:1")
	s.Contains(result.Config.Features, "ghcr.io/devcontainers/features/git:1")
}

func (s *SubstituteTestSuite) TestSubstitute_AdditionalFeaturesOverridesExisting() {
	rawConfig := &config.DevContainerConfig{
		ImageContainer: config.ImageContainer{Image: "alpine:latest"},
		DevContainerConfigBase: config.DevContainerConfigBase{
			Features: map[string]any{
				"ghcr.io/devcontainers/features/node:1": map[string]any{"version": "18"},
			},
		},
	}
	options := provider2.CLIOptions{
		AdditionalFeatures: `{"ghcr.io/devcontainers/features/node:1": {"version": "22"}}`,
	}

	result, _, err := s.runner.substitute(options, rawConfig)

	s.NoError(err)
	s.Len(result.Config.Features, 1)
	nodeOpts, ok := result.Config.Features["ghcr.io/devcontainers/features/node:1"].(map[string]any)
	s.Require().True(ok, "expected feature options to be map[string]any")
	s.Equal("22", nodeOpts["version"])
}

func (s *SubstituteTestSuite) TestSubstitute_AdditionalFeaturesInvalidJSON() {
	rawConfig := &config.DevContainerConfig{
		ImageContainer: config.ImageContainer{Image: "alpine:latest"},
	}
	options := provider2.CLIOptions{
		AdditionalFeatures: `{invalid json`,
	}

	_, _, err := s.runner.substitute(options, rawConfig)

	s.Error(err)
	s.Contains(err.Error(), "--additional-features")
}

func (s *SubstituteTestSuite) TestSubstitute_AdditionalFeaturesEmpty() {
	rawConfig := &config.DevContainerConfig{
		ImageContainer: config.ImageContainer{Image: "alpine:latest"},
	}
	options := provider2.CLIOptions{
		AdditionalFeatures: "",
	}

	result, _, err := s.runner.substitute(options, rawConfig)

	s.NoError(err)
	s.Nil(result.Config.Features)
}

func (s *SubstituteTestSuite) TestSubstitute_ExtraFeatures() {
	const node = "ghcr.io/devcontainers/features/node:1"
	extraPath := filepath.Join(s.T().TempDir(), "extra.json")
	s.Require().NoError(os.WriteFile(extraPath, []byte(`{
		// Personal features.
		"features": {
			"ghcr.io/devcontainers/features/node:1": {"version": "${localEnv:NODE_VERSION}"},
			"ghcr.io/devcontainers/features/git:1": {},
		},
		"forwardPorts": [3774],
	}`), 0o600))

	for _, tc := range []struct {
		name       string
		base       map[string]any
		additional string
		version    string
	}{
		{name: "without base features", version: "20"},
		{
			name: "extra replaces base options",
			base: map[string]any{
				node: map[string]any{"version": "18", "installYarnUsingApt": false},
			},
			version: "20",
		},
		{
			name:       "CLI replaces extra options",
			base:       map[string]any{node: map[string]any{"version": "18"}},
			additional: `{"ghcr.io/devcontainers/features/node:1": {"version": "22"}}`,
			version:    "22",
		},
	} {
		s.Run(tc.name, func() {
			if tc.base != nil {
				tc.base["base-only"] = map[string]any{"enabled": false}
			}
			rawConfig := &config.DevContainerConfig{
				Origin:                 "/workspace/.devcontainer/devcontainer.json",
				DevContainerConfigBase: config.DevContainerConfigBase{Features: tc.base},
			}
			original := config.CloneDevContainerConfig(rawConfig)
			result, _, err := s.runner.substitute(provider2.CLIOptions{
				ExtraDevContainerPath: extraPath,
				AdditionalFeatures:    tc.additional,
				InitEnv:               []string{"NODE_VERSION=20"},
			}, rawConfig)

			s.Require().NoError(err)
			expected := map[string]any{
				node:                                   map[string]any{"version": tc.version},
				"ghcr.io/devcontainers/features/git:1": map[string]any{},
			}
			if tc.base != nil {
				expected["base-only"] = map[string]any{"enabled": false}
			}
			s.Equal(expected, result.Config.Features)
			s.Equal(original, rawConfig)
			s.Equal(original.Origin, result.Config.Origin)
		})
	}
}

func (s *SubstituteTestSuite) TestSubstitute_ExtraFileWithoutFeatures() {
	extraPath := filepath.Join(s.T().TempDir(), "extra.json")
	for _, content := range []string{`{"forwardPorts": [3774]}`, `{"features": {}}`, `{"features": null}`} {
		s.Require().NoError(os.WriteFile(extraPath, []byte(content), 0o600))
		result, _, err := s.runner.substitute(provider2.CLIOptions{
			ExtraDevContainerPath: extraPath,
		}, &config.DevContainerConfig{})
		s.Require().NoError(err)
		s.Nil(result.Config.Features)
	}
}

func (s *SubstituteTestSuite) TestSubstitute_ExtraFeaturesInvalidFile() {
	extraPath := filepath.Join(s.T().TempDir(), "extra.json")
	for _, content := range []string{"", `{invalid`, `{"features": []}`} {
		if content != "" {
			s.Require().NoError(os.WriteFile(extraPath, []byte(content), 0o600))
		}
		_, _, err := s.runner.substitute(provider2.CLIOptions{
			ExtraDevContainerPath: extraPath,
		}, &config.DevContainerConfig{})
		s.Require().Error(err)
		s.Contains(err.Error(), "--extra-devcontainer-path")
	}
}

func (s *SubstituteTestSuite) TestResolveCLIMounts_SubstitutesVariables() {
	substitutionContext := &config.SubstitutionContext{
		DevContainerID:           "test-id",
		LocalWorkspaceFolder:     "/workspace",
		ContainerWorkspaceFolder: testContainerWorkspaceFolder,
		Env: map[string]string{
			"CACHE_NAME": "my-cache=1",
		},
	}

	mounts, err := resolveCLIMounts(substitutionContext, []string{
		`{"type":"volume","source":"${localEnv:CACHE_NAME}","target":"${containerWorkspaceFolder}/cache"}`,
	})

	s.NoError(err)
	s.Len(mounts, 1)
	s.Equal(testVolumeType, mounts[0].Type)
	s.Equal("my-cache=1", mounts[0].Source)
	s.Equal(testContainerWorkspaceFolder+"/cache", mounts[0].Target)
}

func (s *SubstituteTestSuite) TestResolveCLIMounts_AcceptsRawStringForm() {
	substitutionContext := &config.SubstitutionContext{
		ContainerWorkspaceFolder: testContainerWorkspaceFolder,
	}

	mounts, err := resolveCLIMounts(substitutionContext, []string{
		"type=bind,source=/tmp/data,target=${containerWorkspaceFolder}/data,readonly",
	})

	s.NoError(err)
	s.Len(mounts, 1)
	s.Equal("bind", mounts[0].Type)
	s.Equal("/tmp/data", mounts[0].Source)
	s.Equal(testContainerWorkspaceFolder+"/data", mounts[0].Target)
	s.Equal([]string{"readonly"}, mounts[0].Other)
}

func (s *SubstituteTestSuite) TestResolveCLIMounts_RejectsMalformedJSONObjectInput() {
	substitutionContext := &config.SubstitutionContext{}

	_, err := resolveCLIMounts(substitutionContext, []string{
		`{"type":"bind","source":"/tmp/data"`,
	})

	s.Error(err)
	s.Contains(err.Error(), "parse --mount JSON")
}

func (s *SubstituteTestSuite) TestResolveCLIMounts_RequiresTarget() {
	substitutionContext := &config.SubstitutionContext{}

	_, err := resolveCLIMounts(substitutionContext, []string{
		`{"type":"volume","source":"cache"}`,
	})

	s.Error(err)
	s.Contains(err.Error(), "target is required")
}

func (s *SubstituteTestSuite) TestMergeCLIMounts_OverridesByTarget() {
	mergedConfig := &config.MergedDevContainerConfig{
		NonComposeBase: config.NonComposeBase{
			Mounts: []*config.Mount{
				{
					Type:   testVolumeType,
					Source: "existing-cache",
					Target: "/cache",
				},
				{
					Type:   testVolumeType,
					Source: "existing-tools",
					Target: "/tools",
				},
			},
		},
	}
	substitutionContext := &config.SubstitutionContext{
		ContainerWorkspaceFolder: testContainerWorkspaceFolder,
	}

	err := mergeCLIMounts(mergedConfig, substitutionContext, []string{
		`{"type":"volume","source":"cli-cache-old","target":"/cache"}`,
		`{"type":"volume","source":"cli-cache","target":"/cache"}`,
		`{"type":"volume","source":"cli-data","target":"${containerWorkspaceFolder}/data"}`,
	})

	s.NoError(err)
	s.Len(mergedConfig.Mounts, 3)
	s.Equal("existing-tools", mergedConfig.Mounts[0].Source)
	s.Equal("cli-cache", mergedConfig.Mounts[1].Source)
	s.Equal("/cache", mergedConfig.Mounts[1].Target)
	s.Equal("cli-data", mergedConfig.Mounts[2].Source)
	s.Equal(testContainerWorkspaceFolder+"/data", mergedConfig.Mounts[2].Target)
}
