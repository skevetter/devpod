package upgrade

import (
	"context"
	"fmt"
	"os"

	"github.com/creativeprojects/go-selfupdate"
	"github.com/skevetter/log"
)

const defaultRepository = "skevetter/devpod"

// Upgrade downloads the latest release from github and replaces devpod if a new version is found.
// If dryRun is true, it only shows what would be downloaded without actually upgrading.
func Upgrade(targetVersion string, dryRun bool, logger log.Logger) error {
	ctx := context.Background()
	release, err := detectRelease(ctx, targetVersion)
	if err != nil {
		return err
	}

	if dryRun {
		_, _ = fmt.Fprintf(os.Stdout, `asset_name=%s
		version=%s
		os=%s
		arch=%s
		url=%s
		size=%d
		`, release.AssetName, release.Version(), release.OS, release.Arch, release.AssetURL, release.AssetByteSize)
		return nil
	}

	cmdPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	updater, err := selfupdate.NewUpdater(selfupdate.Config{})
	if err != nil {
		return fmt.Errorf("initialize updater: %w", err)
	}

	logger.Infof("downloading version %s", release.Version())
	if err := updater.UpdateTo(ctx, release, cmdPath); err != nil {
		return fmt.Errorf("update to version %s: %w", release.Version(), err)
	}

	logger.Donef("updated devpod to version %s", release.Version())
	return nil
}

// detectRelease detects which release to use based on targetVersion.
func detectRelease(ctx context.Context, targetVersion string) (*selfupdate.Release, error) {
	updater, err := selfupdate.NewUpdater(selfupdate.Config{})
	if err != nil {
		return nil, fmt.Errorf("initialize updater: %w", err)
	}

	repo := selfupdate.ParseSlug(defaultRepository)

	if targetVersion != "" {
		return detectSpecificVersion(ctx, updater, repo, targetVersion)
	}

	return detectLatestVersion(ctx, updater, repo)
}

func detectSpecificVersion(
	ctx context.Context,
	updater *selfupdate.Updater,
	repo selfupdate.RepositorySlug,
	version string,
) (*selfupdate.Release, error) {
	release, found, err := updater.DetectVersion(ctx, repo, version)
	if err != nil {
		return nil, fmt.Errorf("detect version %s: %w", version, err)
	}
	if !found {
		return nil, fmt.Errorf("version %s not found", version)
	}
	return release, nil
}

func detectLatestVersion(
	ctx context.Context,
	updater *selfupdate.Updater,
	repo selfupdate.RepositorySlug,
) (*selfupdate.Release, error) {
	release, found, err := updater.DetectLatest(ctx, repo)
	if err != nil {
		return nil, fmt.Errorf("detect latest version: %w", err)
	}
	if !found {
		return nil, fmt.Errorf("no release found")
	}
	return release, nil
}
