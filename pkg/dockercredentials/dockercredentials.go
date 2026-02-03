package dockercredentials

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/docker/cli/cli/config"
	"github.com/docker/cli/cli/config/types"
	"github.com/kballard/go-shellquote"
	"github.com/skevetter/devpod/pkg/command"
	"github.com/skevetter/devpod/pkg/docker"
	"github.com/skevetter/devpod/pkg/file"
	"github.com/skevetter/devpod/pkg/random"
	"github.com/skevetter/log"

	dockerconfig "github.com/containers/image/v5/pkg/docker/config"
)

type Request struct {
	// If ServerURL is empty its a list request
	ServerURL string
}

type ListResponse struct {
	Registries map[string]string
}

// Credentials holds the information shared between docker and the credentials store.
type Credentials struct {
	ServerURL string
	Username  string
	Secret    string
}

func (c *Credentials) AuthToken() string {
	if c.Username != "" {
		return c.Username + ":" + c.Secret
	}
	return c.Secret
}

const (
	AzureContainerRegistryUsername = "00000000-0000-0000-0000-000000000000"
	windowsOS                      = "windows"
	// #nosec G101 -- this is a helper name, not a credential
	credentialHelperName = "docker-credential-devpod"
)

func getCredentialHelperFilename() string {
	if runtime.GOOS == windowsOS {
		return credentialHelperName + ".cmd"
	}
	return credentialHelperName
}

func getPathSeparator() string {
	if runtime.GOOS == windowsOS {
		return ";"
	}
	return ":"
}

func validateWindowsExecutablePath(path string) error {
	// Validate that path does not contain cmd.exe metacharacters that could break quoting
	unsafeChars := []string{"%", "^", "&", "|", "<", ">", "\"", "\n", "\r", "\t", ";", "(", ")", "!"}
	for _, char := range unsafeChars {
		if strings.Contains(path, char) {
			return fmt.Errorf("executable path contains unsafe character (%s): %s", char, path)
		}
	}
	return nil
}

func writeCredentialHelper(targetDir string, port int, log log.Logger) error {
	binaryPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	helperPath := filepath.Join(targetDir, getCredentialHelperFilename())
	var content []byte

	if runtime.GOOS == windowsOS {
		// Validate path does not contain characters that could break cmd.exe quoting
		if err := validateWindowsExecutablePath(binaryPath); err != nil {
			return err
		}
		content = fmt.Appendf(nil,
			"@echo off\r\n\"%s\" agent docker-credentials --port %d %%*\r\n",
			binaryPath, port,
		)
	} else {
		quotedPath := shellquote.Join(binaryPath)
		content = fmt.Appendf(nil,
			"#!/bin/sh\n%s agent docker-credentials --port %d \"$@\"\n",
			quotedPath, port,
		)
	}

	// #nosec G306 -- credential helper needs to be executable (0755)
	if err := os.WriteFile(helperPath, content, 0755); err != nil {
		return fmt.Errorf("write credential helper: %w", err)
	}

	log.Debugf("wrote docker credentials helper to %s", helperPath)
	return nil
}

func writeCredentialHelperDockerless(targetDir string, port int, log log.Logger) error {
	binaryPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	helperPath := filepath.Join(targetDir, getCredentialHelperFilename())
	quotedPath := shellquote.Join(binaryPath)
	content := fmt.Appendf(nil,
		"#!/.dockerless/bin/sh\n%s agent docker-credentials --port %d \"$@\"\n",
		quotedPath, port,
	)

	// #nosec G306 -- credential helper needs to be executable (0755)
	if err := os.WriteFile(helperPath, content, 0755); err != nil {
		return fmt.Errorf("write credential helper: %w", err)
	}

	log.Debugf("wrote dockerless credentials helper to %s", helperPath)
	return nil
}

func configureDockerConfig(configDir, userName string) error {
	if err := file.MkdirAll(userName, configDir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	dockerConfig, err := config.Load(configDir)
	if err != nil {
		return fmt.Errorf("load docker config: %w", err)
	}

	dockerConfig.CredentialsStore = "devpod"
	if err := dockerConfig.Save(); err != nil {
		return fmt.Errorf("save docker config: %w", err)
	}

	if userName != "" {
		if err := file.Chown(userName, dockerConfig.Filename); err != nil {
			return fmt.Errorf("chown docker config: %w", err)
		}
	}

	return nil
}

func ConfigureCredentialsContainer(userName string, port int, log log.Logger) error {
	log.Debugf("configuring container credentials for user %s", userName)

	userHome, err := command.GetHome(userName)
	if err != nil {
		return fmt.Errorf("get user home: %w", err)
	}

	configDir := getDockerConfigDir(userHome)
	log.Debugf("using docker config directory: %s", configDir)

	if err := setupCredentialHelper(configDir, port, userName, log); err != nil {
		return err
	}

	log.Debugf("container credentials configured")
	return nil
}

func getDockerConfigDir(userHome string) string {
	configDir := os.Getenv("DOCKER_CONFIG")
	if configDir == "" {
		configDir = filepath.Join(userHome, ".docker")
	}
	return configDir
}

func setupCredentialHelper(configDir string, port int, userName string, log log.Logger) error {
	if err := writeCredentialHelper(configDir, port, log); err != nil {
		return err
	}

	if err := os.Setenv("PATH", os.Getenv("PATH")+getPathSeparator()+configDir); err != nil {
		return fmt.Errorf("set PATH: %w", err)
	}

	return configureDockerConfig(configDir, userName)
}

func ConfigureCredentialsDockerless(targetFolder string, port int, log log.Logger) (string, error) {
	dockerConfigDir := filepath.Join(targetFolder, ".cache", random.String(6))
	log.Debugf("configuring dockerless credentials in %s", dockerConfigDir)

	// #nosec G301 -- docker config directory needs to be accessible (0755)
	if err := os.MkdirAll(dockerConfigDir, 0755); err != nil {
		return "", fmt.Errorf("create docker config dir: %w", err)
	}
	log.Debugf("created docker config directory")

	if err := writeCredentialHelperDockerless(dockerConfigDir, port, log); err != nil {
		_ = os.RemoveAll(dockerConfigDir)
		return "", err
	}

	if err := configureDockerConfig(dockerConfigDir, ""); err != nil {
		_ = os.RemoveAll(dockerConfigDir)
		return "", err
	}

	if err := os.Setenv("DOCKER_CONFIG", dockerConfigDir); err != nil {
		_ = os.RemoveAll(dockerConfigDir)
		return "", fmt.Errorf("set DOCKER_CONFIG: %w", err)
	}

	if err := os.Setenv("PATH", os.Getenv("PATH")+getPathSeparator()+dockerConfigDir); err != nil {
		_ = os.RemoveAll(dockerConfigDir)
		return "", fmt.Errorf("set PATH: %w", err)
	}

	log.Debugf("dockerless credentials configured")
	return dockerConfigDir, nil
}

func ConfigureCredentialsMachine(targetFolder string, port int, log log.Logger) (string, error) {
	dockerConfigDir := filepath.Join(targetFolder, ".cache", random.String(12))
	log.Debugf("configuring machine credentials in %s", dockerConfigDir)

	// #nosec G301 -- docker config directory needs to be accessible (0755)
	if err := os.MkdirAll(dockerConfigDir, 0755); err != nil {
		return "", fmt.Errorf("create docker config dir: %w", err)
	}
	log.Debugf("created docker config directory")

	if err := writeCredentialHelper(dockerConfigDir, port, log); err != nil {
		_ = os.RemoveAll(dockerConfigDir)
		return "", err
	}

	if err := configureDockerConfig(dockerConfigDir, ""); err != nil {
		_ = os.RemoveAll(dockerConfigDir)
		return "", err
	}

	if err := os.Setenv("DOCKER_CONFIG", dockerConfigDir); err != nil {
		_ = os.RemoveAll(dockerConfigDir)
		return "", fmt.Errorf("set DOCKER_CONFIG: %w", err)
	}

	if err := os.Setenv("PATH", os.Getenv("PATH")+getPathSeparator()+dockerConfigDir); err != nil {
		_ = os.RemoveAll(dockerConfigDir)
		return "", fmt.Errorf("set PATH: %w", err)
	}

	log.Debugf("machine credentials configured")
	return dockerConfigDir, nil
}

func ListCredentials() (*ListResponse, error) {
	retList := &ListResponse{Registries: map[string]string{}}
	// Get all of the credentials from container tools
	// i.e. podman, skopeo or others
	allContainerCredentials, err := dockerconfig.GetAllCredentials(nil)
	if err != nil {
		return nil, err
	}
	for registryHostname, auth := range allContainerCredentials {
		retList.Registries[registryHostname] = auth.Username
	}

	// get docker credentials
	// if a registry exists twice we overwrite with the docker auth
	// to avoid breaking existing behaviour
	dockerConfig, err := docker.LoadDockerConfig()
	if err != nil {
		return nil, err
	}

	allCredentials, err := dockerConfig.GetAllCredentials()
	if err != nil {
		return nil, err
	}

	for registryHostname, auth := range allCredentials {
		retList.Registries[registryHostname] = auth.Username
	}

	return retList, nil
}

func GetAuthConfig(host string) (*Credentials, error) {
	dockerConfig, err := docker.LoadDockerConfig()
	if err != nil {
		return nil, err
	}

	host = normalizeHost(host)
	ac, err := dockerConfig.GetAuthConfig(host)
	if err != nil {
		return nil, err
	}

	// If Docker config has no credentials, try container ecosystem (podman, skopeo, etc.)
	if isEmptyAuth(ac) {
		ac = getContainerEcosystemAuth(host)
	}

	applyRegistryDefaults(&ac, host)

	return toCredentials(host, ac), nil
}

func normalizeHost(host string) string {
	if host == "registry-1.docker.io" {
		return "https://index.docker.io/v1/"
	}
	return host
}

func isEmptyAuth(ac types.AuthConfig) bool {
	return ac == types.AuthConfig{}
}

func getContainerEcosystemAuth(host string) types.AuthConfig {
	sanitizedHost := strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://")
	dac, err := dockerconfig.GetCredentials(nil, sanitizedHost)
	if err != nil {
		// No credentials available - return empty for anonymous access
		return types.AuthConfig{}
	}

	// Do not return credentials if they are empty (need either username+password OR token)
	if (dac.Username == "" || dac.Password == "") && dac.IdentityToken == "" {
		return types.AuthConfig{}
	}

	return types.AuthConfig{
		Username:      dac.Username,
		Password:      dac.Password,
		IdentityToken: dac.IdentityToken,
		ServerAddress: host,
	}
}

func applyRegistryDefaults(ac *types.AuthConfig, host string) {
	if ac.Username != "" {
		return
	}

	registryAddr := ac.ServerAddress
	if registryAddr == "" {
		registryAddr = host
	}

	registryAddr = strings.TrimPrefix(strings.TrimPrefix(registryAddr, "https://"), "http://")
	if strings.HasSuffix(registryAddr, "azurecr.io") {
		ac.Username = AzureContainerRegistryUsername
	}
}

func toCredentials(host string, ac types.AuthConfig) *Credentials {
	secret := ac.Password
	if ac.IdentityToken != "" {
		secret = ac.IdentityToken
	}

	return &Credentials{
		ServerURL: host,
		Username:  ac.Username,
		Secret:    secret,
	}
}
