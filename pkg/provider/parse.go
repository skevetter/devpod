package provider

import (
	"fmt"
	"io"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/blang/semver/v4"
	"github.com/ghodss/yaml"
	"github.com/skevetter/devpod/pkg/types"
)

var ProviderNameRegEx = regexp.MustCompile(`[^a-z0-9\-]+`)

var optionNameRegEx = regexp.MustCompile(`[^A-Z0-9_]+`)

var allowedTypes = []string{
	"string",
	"multiline",
	"duration",
	"number",
	"boolean",
}

func ParseProvider(reader io.Reader) (*ProviderConfig, error) {
	payload, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	parsedConfig := &ProviderConfig{}
	err = yaml.Unmarshal(payload, parsedConfig)
	if err != nil {
		return nil, fmt.Errorf("parse provider config %w", err)
	}

	err = validate(parsedConfig)
	if err != nil {
		return nil, fmt.Errorf("validate %w", err)
	}

	return parsedConfig, nil
}

func validate(config *ProviderConfig) error {
	// validate name
	if config.Name == "" {
		return fmt.Errorf("name is missing in provider.yaml")
	}
	if ProviderNameRegEx.MatchString(config.Name) {
		return fmt.Errorf("provider name can only include lowercase letters, numbers or dashes")
	} else if len(config.Name) > 32 {
		return fmt.Errorf("provider name cannot be longer than 32 characters")
	}

	// validate version
	if config.Version != "" {
		_, err := semver.Parse(strings.TrimPrefix(config.Version, "v"))
		if err != nil {
			return fmt.Errorf("parse provider version %w", err)
		}
	}

	// validate option names
	for optionName, optionValue := range config.Options {
		if optionNameRegEx.MatchString(optionName) {
			return fmt.Errorf("provider option '%s' can only consist of upper case letters, numbers or underscores. E.g. MY_OPTION, MY_OTHER_OPTION", optionName)
		}

		// validate option validation
		if optionValue.ValidationPattern != "" {
			_, err := regexp.Compile(optionValue.ValidationPattern)
			if err != nil {
				return fmt.Errorf("error parsing validation pattern '%s' for option '%s' %w", optionValue.ValidationPattern, optionName, err)
			}
		}

		if optionValue.Default != "" && optionValue.Command != "" {
			return fmt.Errorf("default and command cannot be used together in option '%s'", optionName)
		}

		if optionValue.Global && optionValue.Cache != "" {
			return fmt.Errorf("global and cache cannot be used together in option '%s'", optionName)
		}

		if optionValue.Global && optionValue.Mutable {
			return fmt.Errorf("global and mutable cannot be used together in option '%s'", optionName)
		}

		if optionValue.Cache != "" {
			_, err := time.ParseDuration(optionValue.Cache)
			if err != nil {
				return fmt.Errorf("invalid cache value for option '%s' %w", optionName, err)
			}
		}

		if optionValue.Type != "" && !contains(allowedTypes, optionValue.Type) {
			return fmt.Errorf("type can only be one of in option '%s': %v", optionName, allowedTypes)
		}

		if optionValue.Cache != "" && optionValue.Command == "" {
			return fmt.Errorf("cache can only be used with command in option '%s'", optionName)
		}
	}

	// validate provider binaries
	err := validateBinaries("binaries", config.Binaries)
	if err != nil {
		return err
	}
	err = validateProviderType(config)
	if err != nil {
		return err
	}

	err = validateOptionGroups(config)
	if err != nil {
		return err
	}

	return nil
}

func validateProviderType(config *ProviderConfig) error {
	if config.IsProxyProvider() {
		return validateProxyProvider(config)
	}

	if config.IsDaemonProvider() {
		return validateDaemonProvider(config)
	}

	return validateStandardProvider(config)
}

func validateProxyProvider(config *ProviderConfig) error {
	if !reflect.DeepEqual(config.Agent, ProviderAgentConfig{}) {
		return fmt.Errorf("agent config is not allowed for proxy providers")
	}

	disallowedExecFields := map[string]types.StrArray{
		"exec.command": config.Exec.Command,
		"exec.create":  config.Exec.Create,
		"exec.start":   config.Exec.Start,
		"exec.stop":    config.Exec.Stop,
		"exec.status":  config.Exec.Status,
		"exec.delete":  config.Exec.Delete,
	}
	for field, value := range disallowedExecFields {
		if len(value) > 0 {
			return fmt.Errorf("%s is not allowed in proxy providers", field)
		}
	}

	requiredProxyFields := map[string]types.StrArray{
		"exec.proxy.status": config.Exec.Proxy.Status,
		"exec.proxy.stop":   config.Exec.Proxy.Stop,
		"exec.proxy.delete": config.Exec.Proxy.Delete,
		"exec.proxy.ssh":    config.Exec.Proxy.Ssh,
		"exec.proxy.up":     config.Exec.Proxy.Up,
	}
	for field, value := range requiredProxyFields {
		if len(value) == 0 {
			return fmt.Errorf("%s is required for proxy providers", field)
		}
	}

	return nil
}

func validateDaemonProvider(config *ProviderConfig) error {
	if !reflect.DeepEqual(config.Agent, ProviderAgentConfig{}) {
		return fmt.Errorf("agent config is not allowed for daemon providers")
	}

	disallowedExecFields := map[string]types.StrArray{
		"exec.command": config.Exec.Command,
		"exec.create":  config.Exec.Create,
		"exec.start":   config.Exec.Start,
		"exec.stop":    config.Exec.Stop,
		"exec.status":  config.Exec.Status,
		"exec.delete":  config.Exec.Delete,
	}
	for field, value := range disallowedExecFields {
		if len(value) > 0 {
			return fmt.Errorf("%s is not allowed in daemon providers", field)
		}
	}

	if len(config.Exec.Daemon.Start) == 0 {
		return fmt.Errorf("exec.daemon.start is required for daemon providers")
	}

	return nil
}

func validateStandardProvider(config *ProviderConfig) error {
	if err := validateAgentDriver(config); err != nil {
		return err
	}

	if err := validateBinaries("agent.binaries", config.Agent.Binaries); err != nil {
		return err
	}

	return validateExecCommands(config)
}

func validateAgentDriver(config *ProviderConfig) error {
	if config.Agent.Driver != "" && config.Agent.Driver != CustomDriver && config.Agent.Driver != DockerDriver && config.Agent.Driver != KubernetesDriver {
		return fmt.Errorf("agent.driver can only be docker, kubernetes or custom")
	}

	if config.Agent.Driver == CustomDriver {
		return validateCustomDriver(config)
	}

	return nil
}

func validateCustomDriver(config *ProviderConfig) error {
	requiredFields := map[string]types.StrArray{
		"agent.custom.targetArchitecture":  config.Agent.Custom.TargetArchitecture,
		"agent.custom.startDevContainer":   config.Agent.Custom.StartDevContainer,
		"agent.custom.stopDevContainer":    config.Agent.Custom.StopDevContainer,
		"agent.custom.runDevContainer":     config.Agent.Custom.RunDevContainer,
		"agent.custom.deleteDevContainer":  config.Agent.Custom.DeleteDevContainer,
		"agent.custom.findDevContainer":    config.Agent.Custom.FindDevContainer,
		"agent.custom.commandDevContainer": config.Agent.Custom.CommandDevContainer,
	}

	for field, value := range requiredFields {
		if len(value) == 0 {
			return fmt.Errorf("%s is required", field)
		}
	}

	return nil
}

func validateExecCommands(config *ProviderConfig) error {
	if len(config.Exec.Command) == 0 {
		return fmt.Errorf("exec.command is required")
	}

	if err := validateExecPair("exec.create", "exec.delete", config.Exec.Create, config.Exec.Delete); err != nil {
		return err
	}

	if err := validateExecPair("exec.start", "exec.stop", config.Exec.Start, config.Exec.Stop); err != nil {
		return err
	}

	if len(config.Exec.Status) == 0 && len(config.Exec.Start) > 0 {
		return fmt.Errorf("exec.status is required")
	}

	if len(config.Exec.Create) == 0 && len(config.Exec.Start) > 0 {
		return fmt.Errorf("exec.create is required")
	}

	return nil
}

func validateExecPair(field1, field2 string, value1, value2 types.StrArray) error {
	if len(value1) > 0 && len(value2) == 0 {
		return fmt.Errorf("%s is required", field2)
	}
	if len(value1) == 0 && len(value2) > 0 {
		return fmt.Errorf("%s is required", field1)
	}
	return nil
}

func validateOptionGroups(config *ProviderConfig) error {
	for idx, group := range config.OptionGroups {
		if group.Name == "" {
			return fmt.Errorf("optionGroups[%d].name cannot be empty", idx)
		}
	}
	return nil
}

func validateBinaries(prefix string, binaries map[string][]*ProviderBinary) error {
	for binaryName, binaryArr := range binaries {
		if optionNameRegEx.MatchString(binaryName) {
			return fmt.Errorf("binary name '%s' can only consist of upper case letters, numbers or underscores. E.g. MY_BINARY, KUBECTL", binaryName)
		}

		for _, binary := range binaryArr {
			if binary.OS != "linux" && binary.OS != "darwin" && binary.OS != "windows" {
				return fmt.Errorf("unsupported binary operating system '%s', must be 'linux', 'darwin' or 'windows'", binary.OS)
			}
			if binary.Path == "" {
				return fmt.Errorf("%s.%s.path required binary path, cannot be empty", prefix, binaryName)
			}
			if binary.ArchivePath == "" && (strings.HasSuffix(binary.Path, ".gz") || strings.HasSuffix(binary.Path, ".tar") || strings.HasSuffix(binary.Path, ".tgz") || strings.HasSuffix(binary.Path, ".zip")) {
				return fmt.Errorf("%s.%s.archivePath required because binary path is an archive", prefix, binaryName)
			}
			if binary.Arch == "" {
				return fmt.Errorf("%s.%s.arch required, cannot be empty", prefix, binaryName)
			}
		}
	}

	return nil
}

func ParseOptions(options []string) (map[string]string, error) {
	retMap := map[string]string{}
	for _, option := range options {
		splitted := strings.Split(option, "=")
		if len(splitted) == 1 {
			return nil, fmt.Errorf("invalid option '%s', expected format KEY=VALUE", option)
		}

		key := strings.ToUpper(strings.TrimSpace(splitted[0]))
		value := strings.Join(splitted[1:], "=")

		retMap[key] = value
	}

	return retMap, nil
}

func contains(haystack []string, needle string) bool {
	return slices.Contains(haystack, needle)
}
