package config

import (
	"os"
	"path/filepath"

	"github.com/skevetter/devpod/pkg/util"
)

// Override devpod home.
const DEVPOD_HOME = "DEVPOD_HOME"

// Override config path.
const DEVPOD_CONFIG = "DEVPOD_CONFIG"

// ConfigDirName is the hidden directory name used for DevPod configuration.
const ConfigDirName = "." + RepoName

// UIEnvVar is the environment variable indicating the desktop UI is active.
const UIEnvVar = "DEVPOD_UI"

// DebugEnvVar is the environment variable to enable debug logging.
const DebugEnvVar = "DEVPOD_DEBUG"

func GetConfigDir() (string, error) {
	homeDir := os.Getenv(DEVPOD_HOME)
	if homeDir != "" {
		return homeDir, nil
	}

	homeDir, err := util.UserHomeDir()
	if err != nil {
		return "", err
	}

	configDir := filepath.Join(homeDir, ConfigDirName)
	return configDir, nil
}

func GetConfigPath() (string, error) {
	configOrigin := os.Getenv(DEVPOD_CONFIG)
	if configOrigin == "" {
		configDir, err := GetConfigDir()
		if err != nil {
			return "", err
		}

		return filepath.Join(configDir, ConfigFile), nil
	}

	return configOrigin, nil
}
