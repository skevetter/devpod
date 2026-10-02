package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/skevetter/devpod/cmd/flags"
	"github.com/skevetter/devpod/pkg/client/clientimplementation"
	"github.com/skevetter/devpod/pkg/config"
	devcontainerconfig "github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/skevetter/devpod/pkg/provider"
	"github.com/skevetter/devpod/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestUpMountFlagIsRepeatableAndPreservesValues(t *testing.T) {
	upCmd := NewUpCmd(&flags.GlobalFlags{})
	args := []string{
		"--mount", `{"type":"volume","source":"cache","target":"/cache"}`,
		"--mount", `type=bind,source=/tmp/data,target=/data,readonly`,
	}

	require.NoError(t, upCmd.ParseFlags(args))

	mounts, err := upCmd.Flags().GetStringArray("mount")
	require.NoError(t, err)
	require.Equal(t, []string{
		`{"type":"volume","source":"cache","target":"/cache"}`,
		`type=bind,source=/tmp/data,target=/data,readonly`,
	}, mounts)
}

func TestExtraDevContainerConfigSurvivesForwardingWithoutLocalFile(t *testing.T) {
	extraPath := filepath.Join(t.TempDir(), "extra.json")
	require.NoError(t, os.WriteFile(extraPath, []byte(`{
		// Personal config stays on the client.
		"features": {"ghcr.io/devcontainers/features/git:1": {"version": "latest"}},
		"forwardPorts": [3774],
	}`), 0o600))
	options := provider.CLIOptions{ExtraDevContainerPath: extraPath}
	require.NoError(t, loadExtraDevContainerConfig(&options))
	require.NoError(t, os.Remove(extraPath))

	// Machine agents receive JSON; proxy providers forward the same options through the environment.
	data, err := json.Marshal(options)
	require.NoError(t, err)
	var remote provider.CLIOptions
	require.NoError(t, json.Unmarshal(data, &remote))
	for name, value := range clientimplementation.EncodeOptions(remote, config.EnvFlagsUp) {
		t.Setenv(name, value)
	}
	remote = provider.CLIOptions{}
	require.NoError(t, mergeDevPodUpOptions(&remote))
	require.NoError(t, loadExtraDevContainerConfig(&remote))
	require.NotNil(t, remote.ExtraDevContainerConfig)
	require.Equal(
		t,
		options.ExtraDevContainerConfig.Features,
		remote.ExtraDevContainerConfig.Features,
	)
	require.Equal(t, types.StrIntArray{"3774"}, remote.ExtraDevContainerConfig.ForwardPorts)
}

func TestLoadExtraDevContainerConfig(t *testing.T) {
	options := provider.CLIOptions{}
	require.NoError(t, loadExtraDevContainerConfig(&options))
	require.Nil(t, options.ExtraDevContainerConfig)

	extraPath := filepath.Join(t.TempDir(), "extra.json")
	options.ExtraDevContainerPath = extraPath
	for _, content := range []string{"", `{invalid`, `{"features": []}`} {
		if content != "" {
			require.NoError(t, os.WriteFile(extraPath, []byte(content), 0o600))
		}
		err := loadExtraDevContainerConfig(&options)
		require.ErrorContains(t, err, "--extra-devcontainer-path")
	}
}

func TestLoadExtraDevContainerConfigResolvesLocalFeatureBeforeForwarding(t *testing.T) {
	extraDir := t.TempDir()
	extraPath := filepath.Join(extraDir, "extra.json")
	require.NoError(t, os.WriteFile(extraPath, []byte(`{"features":{"../feature":{}}}`), 0o600))
	options := provider.CLIOptions{ExtraDevContainerPath: extraPath}
	require.NoError(t, loadExtraDevContainerConfig(&options))

	resolvedFeaturePath, err := filepath.Abs(filepath.Join(extraDir, "../feature"))
	require.NoError(t, err)
	_, ok := options.ExtraDevContainerConfig.Features[resolvedFeaturePath]
	require.True(t, ok)

	data, err := json.Marshal(options)
	require.NoError(t, err)
	var forwarded provider.CLIOptions
	require.NoError(t, json.Unmarshal(data, &forwarded))
	_, ok = forwarded.ExtraDevContainerConfig.Features[resolvedFeaturePath]
	require.True(t, ok)
}

func TestValidateExtraFeatureProvider(t *testing.T) {
	t.Run("remote provider rejects local feature references", func(t *testing.T) {
		options := provider.CLIOptions{
			ExtraDevContainerConfig: &devcontainerconfig.DevContainerConfig{
				DevContainerConfigBase: devcontainerconfig.DevContainerConfigBase{
					Features: map[string]any{
						"../local-feature": map[string]any{},
					},
				},
			},
		}
		remoteProvider := &provider.ProviderConfig{Name: "ssh"}
		require.ErrorContains(
			t,
			validateExtraFeatureProvider(options, remoteProvider),
			"local feature",
		)
		require.ErrorContains(
			t,
			validateExtraFeatureProvider(options, remoteProvider),
			"OCI or HTTP",
		)
	})

	t.Run("remote provider keeps OCI and HTTP features", func(t *testing.T) {
		options := provider.CLIOptions{
			ExtraDevContainerConfig: &devcontainerconfig.DevContainerConfig{
				DevContainerConfigBase: devcontainerconfig.DevContainerConfigBase{
					Features: map[string]any{
						"ghcr.io/devcontainers/features/git:1": map[string]any{},
						"https://example.com/feature.tgz":      map[string]any{},
					},
				},
			},
		}
		remoteProvider := &provider.ProviderConfig{Name: "ssh"}
		require.NoError(t, validateExtraFeatureProvider(options, remoteProvider))
	})

	t.Run("renamed local provider allows local features", func(t *testing.T) {
		options := provider.CLIOptions{
			ExtraDevContainerConfig: &devcontainerconfig.DevContainerConfig{
				DevContainerConfigBase: devcontainerconfig.DevContainerConfigBase{
					Features: map[string]any{"./feature": map[string]any{}},
				},
			},
		}
		localProvider := &provider.ProviderConfig{
			Name:  "personal-tools",
			Agent: provider.ProviderAgentConfig{Local: types.StrBool(config.BoolTrue)},
		}
		require.NoError(t, validateExtraFeatureProvider(options, localProvider))
	})
}
