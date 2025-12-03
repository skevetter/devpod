package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	path2 "path"
	"path/filepath"
	"strings"
	"unicode/utf8"

	doublestar "github.com/bmatcuk/doublestar/v4"
	"github.com/pkg/errors"
	"github.com/tidwall/jsonc"
)

const DEVCONTAINER_FEATURE_FILE_NAME = "devcontainer-feature.json"

func ParseDevContainerFeature(folder string) (*FeatureConfig, error) {
	path := filepath.Join(folder, DEVCONTAINER_FEATURE_FILE_NAME)
	_, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("%s is missing in feature folder", DEVCONTAINER_FEATURE_FILE_NAME)
	}

	path, err = filepath.Abs(path)
	if err != nil {
		return nil, errors.Wrap(err, "make path absolute")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	featureConfig := &FeatureConfig{}
	err = json.Unmarshal(jsonc.ToJSON(data), featureConfig)
	if err != nil {
		return nil, err
	}

	featureConfig.Origin = path
	return featureConfig, nil
}

func SaveDevContainerJSON(config *DevContainerConfig) error {
	if config.Origin == "" {
		return fmt.Errorf("no origin in config")
	}

	err := os.MkdirAll(filepath.Dir(config.Origin), 0755)
	if err != nil {
		return err
	}

	out, err := json.Marshal(config)
	if err != nil {
		return err
	}

	err = os.WriteFile(config.Origin, out, 0644)
	if err != nil {
		return err
	}

	return nil
}

// ParseDevContainerJSONFile parse the given a devcontainer.json file.
func ParseDevContainerJSONFile(jsonFilePath string) (*DevContainerConfig, error) {
	var err error
	path, err := filepath.Abs(jsonFilePath)
	if err != nil {
		return nil, errors.Wrap(err, "make path absolute")
	}

	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	devContainer := &DevContainerConfig{}
	err = json.Unmarshal(jsonc.ToJSON(bytes), devContainer)
	if err != nil {
		return nil, err
	}
	devContainer.Origin = path
	return replaceLegacy(devContainer)
}

// ParseDevContainerJSON check if a file named devcontainer.json exists in the given directory and parse it if it does
func ParseDevContainerJSON(folder, relativePath string) (*DevContainerConfig, error) {
	return ParseDevContainerJSONWithSelector(folder, relativePath, nil)
}

// ParseDevContainerJSONWithSelector allows custom selection when multiple devcontainer configs exist
func ParseDevContainerJSONWithSelector(folder, relativePath string, selector func([]string) (string, error)) (*DevContainerConfig, error) {
	path, err := resolveDevContainerPath(folder, relativePath, selector)
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, nil
	}
	return ParseDevContainerJSONFile(path)
}

func resolveDevContainerPath(folder, relativePath string, selector func([]string) (string, error)) (string, error) {
	// Explicit path provided
	if relativePath != "" {
		path := path2.Join(filepath.ToSlash(folder), relativePath)
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("devcontainer path %s doesn't exist: %w", path, err)
		}
		return path, nil
	}

	// Try .devcontainer/devcontainer.json
	path := filepath.Join(folder, ".devcontainer", "devcontainer.json")
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}

	// Try .devcontainer.json
	path = filepath.Join(folder, ".devcontainer.json")
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}

	// Find multiple configs
	matches, err := findDevContainerConfigs(folder)
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", nil
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if selector == nil {
		return matches[0], nil
	}
	return selector(matches)
}

func findDevContainerConfigs(folder string) ([]string, error) {
	var configs []string

	// Check .devcontainer/FOLDER/devcontainer.json (one level deep)
	// https://containers.dev/implementors/spec/#devcontainerjson
	devcontainerDir := filepath.Join(folder, ".devcontainer")
	entries, err := os.ReadDir(devcontainerDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				configPath := filepath.Join(devcontainerDir, entry.Name(), "devcontainer.json")
				if _, err := os.Stat(configPath); err == nil {
					configs = append(configs, configPath)
				}
			}
		}
	}

	// Fallback to glob for deeper structures
	if len(configs) == 0 {
		matches, err := doublestar.FilepathGlob(filepath.ToSlash(filepath.Clean(folder)) + "/.devcontainer/**/devcontainer.json")
		if err != nil {
			return nil, err
		}
		configs = matches
	}

	return configs, nil
}

// ListDevContainerIDs returns available devcontainer IDs in the folder
func ListDevContainerIDs(folder string) ([]string, error) {
	configs, err := findDevContainerConfigs(folder)
	if err != nil {
		return nil, err
	}

	var ids []string
	for _, config := range configs {
		id := filepath.Base(filepath.Dir(config))
		if id != ".devcontainer" {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func replaceLegacy(config *DevContainerConfig) (*DevContainerConfig, error) {
	if len(config.Extensions) == 0 && len(config.Settings) == 0 && config.DevPort == 0 {
		return config, nil
	}

	// make sure customizations exist
	if config.Customizations == nil {
		config.Customizations = map[string]interface{}{}
	}

	vsCodeConfig := &VSCodeCustomizations{}
	vscode, ok := config.Customizations["vscode"]
	if ok {
		err := Convert(vscode, &vsCodeConfig)
		if err != nil {
			return nil, err
		}
	}

	if len(config.Extensions) > 0 {
		vsCodeConfig.Extensions = config.Extensions
		config.Extensions = nil
	}

	if len(config.Settings) > 0 {
		if vsCodeConfig.Settings == nil {
			vsCodeConfig.Settings = map[string]interface{}{}
		}

		for k, v := range config.Settings {
			_, exists := vsCodeConfig.Settings[k]
			if !exists {
				vsCodeConfig.Settings[k] = v
			}
		}

		config.Settings = nil
	}

	if vsCodeConfig.DevPort == 0 {
		vsCodeConfig.DevPort = config.DevPort
		config.DevPort = 0
	}

	config.Customizations["vscode"] = vsCodeConfig
	return config, nil
}

func Convert(from interface{}, to interface{}) error {
	out, err := json.Marshal(from)
	if err != nil {
		return err
	}

	return json.Unmarshal(out, to)
}

func ParseKeyValueFile(filename string) ([]string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	keyValuePairs := []string{}
	scanner := bufio.NewScanner(f)
	lineNum := 1
	for scanner.Scan() {
		scannedBytes := scanner.Bytes()
		if !utf8.Valid(scannedBytes) {
			return nil, fmt.Errorf("env file %s contains invalid utf8 bytes in line %d", filename, lineNum)
		}
		line := string(scannedBytes)
		// skip commented or empty lines
		if len(line) > 0 && !strings.HasPrefix(line, "#") {
			key, value, found := strings.Cut(line, "=")
			if len(key) == 0 || strings.Contains(key, " ") {
				return nil, fmt.Errorf("env file %s contains invalid variable key in line %d: %s", filename, lineNum, line)
			} else if len(value) == 0 {
				return nil, fmt.Errorf("env file %s contains invalid variable value in line %d: %s", filename, lineNum, line)
			}
			if found {
				keyValuePairs = append(keyValuePairs, line)
			}
		}
		lineNum++
	}
	return keyValuePairs, nil
}
