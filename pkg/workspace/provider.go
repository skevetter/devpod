package workspace

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	devpodhttp "github.com/skevetter/devpod/pkg/http"
	providerpkg "github.com/skevetter/devpod/pkg/provider"
	"github.com/skevetter/devpod/pkg/types"
	"github.com/skevetter/devpod/providers"

	"errors"

	"github.com/skevetter/devpod/pkg/binaries"
	"github.com/skevetter/devpod/pkg/config"
	"github.com/skevetter/devpod/pkg/download"
	"github.com/skevetter/log"
)

const (
	httpPrefix     = "http://"
	httpsPrefix    = "https://"
	githubPrefix   = "github.com/"
	yamlExt        = ".yaml"
	ymlExt         = ".yml"
	dsStorePrefix  = ".DS_Store"
	providerPrefix = "skevetter/devpod-provider-"
)

var (
	ErrNoWorkspaceFound = errors.New("no workspace found")
)

type ProviderWithOptions struct {
	Config *providerpkg.ProviderConfig `json:"config,omitempty"`
	State  *config.ProviderConfig      `json:"state,omitempty"`
}

type ProviderParams struct {
	DevPodConfig *config.Config
	ProviderName string
	Raw          []byte
	Source       *providerpkg.ProviderSource
	Log          log.Logger
}

// LoadProviders loads all known providers for the given context.
func LoadProviders(
	devPodConfig *config.Config,
	log log.Logger,
) (*ProviderWithOptions, map[string]*ProviderWithOptions, error) {
	defaultContext := devPodConfig.Current()
	retProviders, err := LoadAllProviders(devPodConfig, log)
	if err != nil {
		return nil, nil, err
	}

	if defaultContext.DefaultProvider == "" {
		return nil, nil, fmt.Errorf("no default provider found")
	}
	if retProviders[defaultContext.DefaultProvider] == nil {
		return nil, nil, fmt.Errorf("provider with name %s not found", defaultContext.DefaultProvider)
	}

	return retProviders[defaultContext.DefaultProvider], retProviders, nil
}

func LoadAllProviders(devPodConfig *config.Config, log log.Logger) (map[string]*ProviderWithOptions, error) {
	retProviders := map[string]*ProviderWithOptions{}

	loadConfiguredProviders(devPodConfig, retProviders, log)

	if err := loadUnconfiguredProviders(devPodConfig, retProviders); err != nil {
		return nil, err
	}

	return retProviders, nil
}

func FindProvider(devPodConfig *config.Config, name string, log log.Logger) (*ProviderWithOptions, error) {
	retProviders, err := LoadAllProviders(devPodConfig, log)
	if err != nil {
		return nil, err
	}
	if retProviders[name] == nil {
		return nil, fmt.Errorf("provider with name %s not found", name)
	}

	return retProviders[name], nil
}

func ProviderFromHost(
	ctx context.Context,
	devPodConfig *config.Config,
	proHost string,
	log log.Logger,
) (*providerpkg.ProviderConfig, error) {
	proInstanceConfig, err := providerpkg.LoadProInstanceConfig(devPodConfig.DefaultContext, proHost)
	if err != nil {
		return nil, fmt.Errorf("load pro instance %s: %w", proHost, err)
	}

	provider, err := FindProvider(devPodConfig, proInstanceConfig.Provider, log)
	if err != nil {
		return nil, fmt.Errorf("find provider: %w", err)
	}
	if !provider.Config.IsProxyProvider() && !provider.Config.IsDaemonProvider() {
		return nil, fmt.Errorf("provider is not a pro provider")
	}

	return provider.Config, nil
}

func AddProvider(
	devPodConfig *config.Config,
	providerName, providerSourceRaw string,
	log log.Logger,
) (*providerpkg.ProviderConfig, error) {
	providerRaw, providerSource, err := ResolveProvider(providerSourceRaw, log)
	if err != nil {
		return nil, err
	}

	return AddProviderRaw(ProviderParams{
		DevPodConfig: devPodConfig,
		ProviderName: providerName,
		Source:       providerSource,
		Raw:          providerRaw,
		Log:          log,
	})
}

func AddProviderRaw(p ProviderParams) (*providerpkg.ProviderConfig, error) {
	providerConfig, err := installRawProvider(p)
	if err != nil {
		return nil, err
	}

	if p.DevPodConfig.Current().Providers == nil {
		p.DevPodConfig.Current().Providers = map[string]*config.ProviderConfig{}
	}
	if p.DevPodConfig.Current().Providers[providerConfig.Name] == nil {
		p.DevPodConfig.Current().Providers[providerConfig.Name] = &config.ProviderConfig{
			CreationTimestamp: types.Now(),
		}
	}

	if err := config.SaveConfig(p.DevPodConfig); err != nil {
		return nil, fmt.Errorf("save config: %w", err)
	}

	return providerConfig, nil
}

func UpdateProvider(
	devPodConfig *config.Config,
	providerName, providerSourceRaw string,
	log log.Logger,
) (*providerpkg.ProviderConfig, error) {
	if devPodConfig.Current().Providers[providerName] == nil {
		return nil, fmt.Errorf("provider %s not found", providerName)
	}

	if providerSourceRaw == "" {
		s, err := ResolveProviderSource(devPodConfig, providerName, log)
		if err != nil {
			return nil, err
		}
		providerSourceRaw = s
	}

	providerRaw, providerSource, err := ResolveProvider(providerSourceRaw, log)
	if err != nil {
		return nil, err
	}

	return updateProvider(ProviderParams{
		DevPodConfig: devPodConfig,
		ProviderName: providerName,
		Raw:          providerRaw,
		Source:       providerSource,
		Log:          log,
	})
}

func CloneProvider(
	devPodConfig *config.Config,
	providerName, providerSourceRaw string,
	log log.Logger,
) (*ProviderWithOptions, error) {
	sourceProvider, err := FindProvider(devPodConfig, providerSourceRaw, log)
	if err != nil {
		return nil, err
	}

	providerConfig, err := installProvider(
		ProviderParams{
			DevPodConfig: devPodConfig,
			ProviderName: providerName,
			Source:       &sourceProvider.Config.Source,
			Log:          log,
		},
		sourceProvider.Config)
	if err != nil {
		return nil, err
	}
	sourceProvider.Config = providerConfig

	return sourceProvider, nil
}

func ResolveProviderSource(devPodConfig *config.Config, providerName string, log log.Logger) (string, error) {
	providerConfig, err := FindProvider(devPodConfig, providerName, log)
	if err != nil {
		return "", fmt.Errorf("find provider: %w", err)
	}

	source := getProviderSource(providerConfig.Config.Source, providerConfig.Config.Name)
	if source == "" {
		return "", fmt.Errorf("provider %s source is missing", providerName)
	}

	return source, nil
}

func ResolveProvider(providerSource string, log log.Logger) ([]byte, *providerpkg.ProviderSource, error) {
	retSource := &providerpkg.ProviderSource{Raw: strings.TrimSpace(providerSource)}

	if out, ok := resolveInternalProvider(providerSource, retSource); ok {
		return out, retSource, nil
	}

	if out, ok, err := resolveURLProvider(providerSource, retSource, log); ok {
		if err != nil {
			return nil, nil, err
		}
		return out, retSource, nil
	}

	if out, ok := resolveFileProvider(providerSource, retSource); ok {
		return out, retSource, nil
	}

	out, source, err := downloadProviderGithub(providerSource, log)
	if err != nil {
		return nil, nil, fmt.Errorf("download github: %w", err)
	}
	if len(out) > 0 {
		return out, source, nil
	}

	return nil, nil, fmt.Errorf("provider type not recognized: specify a local file, url, or github repository")
}

func downloadProviderGithub(originalPath string, log log.Logger) ([]byte, *providerpkg.ProviderSource, error) {
	path := strings.TrimPrefix(originalPath, githubPrefix)

	release := ""
	index := strings.LastIndex(path, "@")
	if index != -1 {
		release = path[index+1:]
		path = path[:index]
	}

	splitted := strings.Split(strings.TrimSuffix(path, "/"), "/")
	if len(splitted) == 1 {
		path = providerPrefix + path
	} else if len(splitted) != 2 {
		return nil, nil, nil
	}

	requestURL := buildGithubURL(path, release)

	body, err := download.File(requestURL, log)
	if err != nil {
		return nil, nil, fmt.Errorf("download: %w", err)
	}
	defer func() { _ = body.Close() }()

	out, err := io.ReadAll(body)
	if err != nil {
		return nil, nil, err
	}

	return out, &providerpkg.ProviderSource{
		Raw:    originalPath,
		Github: path,
	}, nil
}

func loadConfiguredProviders(
	devPodConfig *config.Config,
	retProviders map[string]*ProviderWithOptions,
	log log.Logger,
) {
	defaultContext := devPodConfig.Current()
	for providerName, providerState := range defaultContext.Providers {
		if retProviders[providerName] != nil {
			retProviders[providerName].State = providerState
			continue
		}

		providerConfig, err := providerpkg.LoadProviderConfig(devPodConfig.DefaultContext, providerName)
		if err != nil {
			log.Warnf("error loading provider %s: %v", providerName, err)
			continue
		}

		retProviders[providerName] = &ProviderWithOptions{
			Config: providerConfig,
			State:  providerState,
		}
	}
}

func loadUnconfiguredProviders(devPodConfig *config.Config, retProviders map[string]*ProviderWithOptions) error {
	providerDir, err := providerpkg.GetProvidersDir(devPodConfig.DefaultContext)
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(providerDir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	for _, entry := range entries {
		if shouldSkipEntry(entry, retProviders) {
			continue
		}

		if err := loadProviderEntry(devPodConfig, entry, retProviders); err != nil {
			return err
		}
	}

	return nil
}

func shouldSkipEntry(entry os.DirEntry, retProviders map[string]*ProviderWithOptions) bool {
	return retProviders[entry.Name()] != nil || !entry.IsDir() || strings.HasPrefix(entry.Name(), dsStorePrefix)
}

func loadProviderEntry(
	devPodConfig *config.Config,
	entry os.DirEntry,
	retProviders map[string]*ProviderWithOptions,
) error {
	providerConfig, err := providerpkg.LoadProviderConfig(devPodConfig.DefaultContext, entry.Name())
	if err != nil {
		return err
	}

	retProviders[providerConfig.Name] = &ProviderWithOptions{
		Config: providerConfig,
	}

	return nil
}

func installRawProvider(p ProviderParams) (*providerpkg.ProviderConfig, error) {
	providerConfig, err := providerpkg.ParseProvider(bytes.NewReader(p.Raw))
	if err != nil {
		return nil, err
	}
	return installProvider(ProviderParams{
		DevPodConfig: p.DevPodConfig,
		ProviderName: p.ProviderName,
		Source:       p.Source,
		Log:          p.Log,
	}, providerConfig)
}

func installProvider(
	p ProviderParams,
	providerConfig *providerpkg.ProviderConfig,
) (*providerpkg.ProviderConfig, error) {
	providerConfig.Source = *p.Source
	if p.ProviderName != "" {
		providerConfig.Name = p.ProviderName
	}

	if err := checkProviderNotExists(p.DevPodConfig, providerConfig.Name); err != nil {
		return nil, err
	}

	if err := downloadAndSaveProvider(p, providerConfig); err != nil {
		return nil, err
	}

	return providerConfig, nil
}

func updateProvider(p ProviderParams) (*providerpkg.ProviderConfig, error) {
	providerConfig, err := providerpkg.ParseProvider(bytes.NewReader(p.Raw))
	if err != nil {
		return nil, err
	}

	providerConfig.Source = *p.Source
	if p.ProviderName != "" {
		providerConfig.Name = p.ProviderName
	}
	if providerConfig.Options == nil {
		providerConfig.Options = map[string]*types.Option{}
	}

	cleanupOldOptions(p.DevPodConfig, providerConfig)

	if err := config.SaveConfig(p.DevPodConfig); err != nil {
		return nil, err
	}

	if err := downloadProviderBinaries(p, providerConfig); err != nil {
		return nil, err
	}

	if err := providerpkg.SaveProviderConfig(p.DevPodConfig.DefaultContext, providerConfig); err != nil {
		return nil, err
	}

	return providerConfig, nil
}

func checkProviderNotExists(devPodConfig *config.Config, providerName string) error {
	if devPodConfig.Current().Providers[providerName] != nil {
		return fmt.Errorf("provider %s already exists", providerName)
	}

	providerDir, err := providerpkg.GetProviderDir(devPodConfig.DefaultContext, providerName)
	if err != nil {
		return err
	}

	if _, err := os.Stat(providerDir); err == nil {
		return fmt.Errorf("provider %s already exists", providerName)
	}

	return nil
}

func downloadAndSaveProvider(p ProviderParams, providerConfig *providerpkg.ProviderConfig) error {
	binariesDir, err := providerpkg.GetProviderBinariesDir(p.DevPodConfig.DefaultContext, providerConfig.Name)
	if err != nil {
		return fmt.Errorf("get binaries dir: %w", err)
	}

	providerDir, err := providerpkg.GetProviderDir(p.DevPodConfig.DefaultContext, providerConfig.Name)
	if err != nil {
		return fmt.Errorf("get provider dir: %w", err)
	}

	if _, err := binaries.DownloadBinaries(providerConfig.Binaries, binariesDir, p.Log); err != nil {
		_ = os.RemoveAll(providerDir)
		return fmt.Errorf("download binaries: %w", err)
	}

	return providerpkg.SaveProviderConfig(p.DevPodConfig.DefaultContext, providerConfig)
}

func cleanupOldOptions(devPodConfig *config.Config, providerConfig *providerpkg.ProviderConfig) {
	for optionName := range devPodConfig.Current().Providers[providerConfig.Name].Options {
		if _, ok := providerConfig.Options[optionName]; !ok {
			delete(devPodConfig.Current().Providers[providerConfig.Name].Options, optionName)
		}
	}
}

func downloadProviderBinaries(p ProviderParams, providerConfig *providerpkg.ProviderConfig) error {
	binariesDir, err := providerpkg.GetProviderBinariesDir(p.DevPodConfig.DefaultContext, providerConfig.Name)
	if err != nil {
		return fmt.Errorf("get binaries dir: %w", err)
	}

	if _, err := binaries.DownloadBinaries(providerConfig.Binaries, binariesDir, p.Log); err != nil {
		_ = os.RemoveAll(binariesDir)
		return fmt.Errorf("download binaries: %w", err)
	}

	return nil
}

func getProviderSource(src providerpkg.ProviderSource, configName string) string {
	switch {
	case src.Internal:
		if src.Raw == "" {
			return configName
		}
		return src.Raw
	case src.URL != "":
		return src.URL
	case src.File != "":
		return src.File
	case src.Github != "":
		return src.Github
	default:
		return ""
	}
}

func resolveInternalProvider(providerSource string, retSource *providerpkg.ProviderSource) ([]byte, bool) {
	internalProviders := providers.GetBuiltInProviders()
	if internalProviders[providerSource] != "" {
		retSource.Internal = true
		return []byte(internalProviders[providerSource]), true
	}
	return nil, false
}

func resolveURLProvider(
	providerSource string,
	retSource *providerpkg.ProviderSource,
	log log.Logger,
) ([]byte, bool, error) {
	if !strings.HasPrefix(providerSource, httpPrefix) && !strings.HasPrefix(providerSource, httpsPrefix) {
		return nil, false, nil
	}

	log.Infof("downloading provider from %s", providerSource)
	out, err := downloadProvider(providerSource)
	if err != nil {
		return nil, true, fmt.Errorf("download provider: %w", err)
	}
	retSource.URL = providerSource
	return out, true, nil
}

func resolveFileProvider(providerSource string, retSource *providerpkg.ProviderSource) ([]byte, bool) {
	if !strings.HasSuffix(providerSource, yamlExt) && !strings.HasSuffix(providerSource, ymlExt) {
		return nil, false
	}

	if _, err := os.Stat(providerSource); err != nil {
		return nil, false
	}

	// #nosec G304 - providerSource is user-provided path for loading provider config
	out, err := os.ReadFile(providerSource)
	if err != nil {
		return nil, false
	}

	absPath, err := filepath.Abs(providerSource)
	if err != nil {
		return nil, false
	}
	retSource.File = absPath
	return out, true
}

func downloadProvider(url string) ([]byte, error) {
	resp, err := devpodhttp.GetHTTPClient().Get(url)
	if err != nil {
		return nil, fmt.Errorf("download binary: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func buildGithubURL(path, release string) string {
	if release == "" {
		return fmt.Sprintf("https://github.com/%s/releases/latest/download/provider.yaml", path)
	}
	return fmt.Sprintf("https://github.com/%s/releases/download/%s/provider.yaml", path, release)
}
