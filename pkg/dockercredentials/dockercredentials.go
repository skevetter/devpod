package dockercredentials

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/docker/cli/cli/config"
	"github.com/docker/cli/cli/config/types"
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

func writeCredentialHelper(targetDir string, port int, log log.Logger) error {
	binaryPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	helperPath := filepath.Join(targetDir, getCredentialHelperFilename())
	var content []byte

	if runtime.GOOS == windowsOS {
		content = fmt.Appendf(nil,
			"@echo off\r\n\"%s\" agent docker-credentials --port %d %%*\r\n",
			binaryPath, port,
		)
	} else {
		content = fmt.Appendf(nil,
			"#!/bin/sh\n'%s' agent docker-credentials --port '%d' \"$@\"\n",
			binaryPath, port,
		)
	}

	// #nosec G306 -- credential helper needs to be executable (0755)
	if err := os.WriteFile(helperPath, content, 0755); err != nil {
		return fmt.Errorf("write credential helper: %w", err)
	}

	log.Debugf("wrote docker credentials helper to %s", helperPath)
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

	configDir := os.Getenv("DOCKER_CONFIG")
	if configDir == "" {
		configDir = filepath.Join(userHome, ".docker")
	}
	log.Debugf("using docker config directory: %s", configDir)

	targetDir := "/usr/local/bin"
	if runtime.GOOS == windowsOS {
		targetDir = configDir
	}

	if err := writeCredentialHelper(targetDir, port, log); err != nil {
		return err
	}

	if err := configureDockerConfig(configDir, userName); err != nil {
		return err
	}

	log.Debugf("container credentials configured")
	return nil
}

func ConfigureCredentialsDockerless(targetFolder string, port int, log log.Logger) (string, error) {
	dockerConfigDir := filepath.Join(targetFolder, ".cache", random.String(6))
	log.Debugf("configuring dockerless credentials in %s", dockerConfigDir)

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

	helperPath := filepath.Join(dockerConfigDir, getCredentialHelperFilename())
	if _, err := os.Stat(helperPath); err != nil {
		_ = os.RemoveAll(dockerConfigDir)
		return "", fmt.Errorf("credential helper not found at %s: %w", helperPath, err)
	}
	log.Debugf("credential helper exists at %s", helperPath)
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

	applyRegistryDefaults(&ac)

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

func applyRegistryDefaults(ac *types.AuthConfig) {
	// Azure Container Registry requires a default username
	if ac.Username == "" && strings.HasSuffix(ac.ServerAddress, "azurecr.io") {
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
