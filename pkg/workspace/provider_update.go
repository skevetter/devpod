package workspace

import (
	"fmt"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/skevetter/devpod/pkg/config"
	"github.com/skevetter/devpod/pkg/platform"
	provider2 "github.com/skevetter/devpod/pkg/provider"
	"github.com/skevetter/devpod/pkg/version"
	"github.com/skevetter/log"
)

// CheckProviderUpdate currently only ensures the local provider is in sync with the remote for DevPod Pro instances.
// Potentially auto-upgrade other providers in the future.
func CheckProviderUpdate(
	devPodConfig *config.Config,
	proInstance *provider2.ProInstance,
	log log.Logger,
) error {
	if version.GetVersion() == version.DevVersion {
		log.Debugf("skipping provider upgrade check during development")
		return nil
	}
	if proInstance == nil {
		log.Debug("no pro instance available, skipping provider upgrade check")
		return nil
	}

	return checkProviderUpdateForInstance(devPodConfig, proInstance, log)
}

func checkProviderUpdateForInstance(
	devPodConfig *config.Config,
	proInstance *provider2.ProInstance,
	log log.Logger,
) error {
	newVersion, err := platform.GetProInstanceDevPodVersion(proInstance)
	if err != nil {
		return fmt.Errorf("version for pro instance %s: %w", proInstance.Host, err)
	}

	p, err := FindProvider(devPodConfig, proInstance.Provider, log)
	if err != nil {
		return fmt.Errorf("get provider config for pro provider %s: %w", proInstance.Provider, err)
	}

	if shouldSkipProviderUpdate(p.Config.Version == version.DevVersion, p.Config.Source.Internal) {
		return nil
	}

	needsUpdate, err := providerVersionNeedsUpdate(newVersion, p.Config.Version)
	if err != nil {
		return err
	}
	if !needsUpdate {
		return nil
	}

	log.Infof(
		"New provider version available, attempting to update %s from %s to %s",
		proInstance.Provider,
		p.Config.Version,
		newVersion,
	)

	return applyProviderUpdate(devPodConfig, proInstance.Provider, newVersion, log)
}

func providerVersionNeedsUpdate(newVersion, currentVersion string) (bool, error) {
	v1, err := semver.Parse(strings.TrimPrefix(newVersion, "v"))
	if err != nil {
		return false, fmt.Errorf("parse version %s: %w", newVersion, err)
	}
	v2, err := semver.Parse(strings.TrimPrefix(currentVersion, "v"))
	if err != nil {
		return false, fmt.Errorf("parse version %s: %w", currentVersion, err)
	}
	return v1.Compare(v2) != 0, nil
}

func applyProviderUpdate(
	devPodConfig *config.Config,
	providerName, newVersion string,
	log log.Logger,
) error {
	providerSource, err := ResolveProviderSource(devPodConfig, providerName, log)
	if err != nil {
		return fmt.Errorf("resolve provider source %s: %w", providerName, err)
	}

	splitted := strings.Split(providerSource, "@")
	if len(splitted) == 0 {
		return fmt.Errorf("no provider source found %s", providerSource)
	}
	providerSource = splitted[0] + "@" + newVersion

	_, err = UpdateProvider(devPodConfig, providerName, providerSource, log)
	if err != nil {
		return fmt.Errorf("update provider %s: %w", providerName, err)
	}

	log.Donef("updated provider: provider=%s", providerName)
	return nil
}

// GetProInstance returns the ProInstance associated with the given provider name, or nil if not found.
func GetProInstance(
	devPodConfig *config.Config,
	providerName string,
	log log.Logger,
) *provider2.ProInstance {
	proInstances, err := ListProInstances(devPodConfig, log)
	if err != nil {
		return nil
	} else if len(proInstances) == 0 {
		return nil
	}

	proInstance, ok := FindProviderProInstance(proInstances, providerName)
	if !ok {
		return nil
	}

	return proInstance
}

// shouldSkipProviderUpdate returns true if the provider update check should be skipped.
func shouldSkipProviderUpdate(isDevVersion, isInternal bool) bool {
	return isDevVersion || isInternal
}
