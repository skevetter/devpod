package upgrade

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/blang/semver/v4"
	"github.com/creativeprojects/go-selfupdate"
	versionpkg "github.com/skevetter/devpod/pkg/version"
	"github.com/skevetter/log"
)

const defaultRepository = "skevetter/devpod"

var (
	currentVersion     = strings.TrimPrefix(versionpkg.GetVersion(), "v")
	developmentVersion = strings.TrimPrefix(versionpkg.DevVersion, "v")
)

// Upgrade downloads the latest release from github and replaces devpod if a new version is found.
func Upgrade(targetVersion string, logger log.Logger) error {
	updater, err := selfupdate.NewUpdater(selfupdate.Config{})
	if err != nil {
		return fmt.Errorf("initialize updater: %w", err)
	}

	if targetVersion != "" {
		return upgradeToVersion(context.Background(), updater, targetVersion, logger)
	}

	return upgradeToLatest(context.Background(), updater, logger)
}

// checker handles version checking operations.
type checker struct {
	repository    string
	currentVer    string
	devVer        string
	latestVersion string
	checkErr      error
	once          sync.Once
}

// newChecker creates a new version checker.
func newChecker() *checker {
	return &checker{
		repository: defaultRepository,
		currentVer: currentVersion,
		devVer:     developmentVersion,
	}
}

// checkLatest checks if there is a newer version available.
func (c *checker) checkLatest(ctx context.Context) (string, error) {
	c.once.Do(func() {
		latest, found, err := selfupdate.DetectLatest(ctx, selfupdate.ParseSlug(c.repository))
		if err != nil {
			c.checkErr = fmt.Errorf("detect latest version: %w", err)
			return
		}

		if !found || latest.Equal(c.currentVer) {
			return
		}

		c.latestVersion = latest.Version()
	})

	return c.latestVersion, c.checkErr
}

// isNewerAvailable returns the newer version if available, empty string otherwise.
func (c *checker) isNewerAvailable(ctx context.Context) (string, error) {
	if c.currentVer == c.devVer {
		return "", nil
	}

	if c.currentVer == "" {
		return "", nil
	}

	latestVer, err := c.checkLatest(ctx)
	if err != nil || latestVer == "" {
		return "", err
	}

	current, err := semver.Parse(c.currentVer)
	if err != nil {
		return "", fmt.Errorf("parse current version: %w", err)
	}

	latest, err := semver.Parse(latestVer)
	if err != nil {
		return "", fmt.Errorf("parse latest version: %w", err)
	}

	if latest.Compare(current) == 1 {
		return latestVer, nil
	}

	return "", nil
}

func upgradeToVersion(ctx context.Context, updater *selfupdate.Updater, version string, logger log.Logger) error {
	release, found, err := updater.DetectVersion(ctx, selfupdate.ParseSlug(defaultRepository), version)
	if err != nil {
		return fmt.Errorf("detect version %s: %w", version, err)
	}
	if !found {
		return fmt.Errorf("version %s not found", version)
	}

	cmdPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	logger.Infof("downloading version %s", version)
	if err := updater.UpdateTo(ctx, release, cmdPath); err != nil {
		return fmt.Errorf("update to version %s: %w", version, err)
	}

	logger.Donef("updated devpod to version %s", version)
	return nil
}

func upgradeToLatest(ctx context.Context, updater *selfupdate.Updater, logger log.Logger) error {
	checker := newChecker()
	newerVersion, err := checker.isNewerAvailable(ctx)
	if err != nil {
		return fmt.Errorf("check for newer version: %w", err)
	}

	if newerVersion == "" {
		logger.Infof("current binary is the latest version %s", currentVersion)
		return nil
	}

	logger.Info("downloading newest version")
	latest, err := updater.UpdateSelf(ctx, currentVersion, selfupdate.ParseSlug(defaultRepository))
	if err != nil {
		return fmt.Errorf("update to latest: %w", err)
	}

	if latest.Equal(currentVersion) {
		logger.Donef("current binary is the latest version %s", currentVersion)
		return nil
	}

	logger.Donef("updated to version %s", latest.Version())
	return nil
}
