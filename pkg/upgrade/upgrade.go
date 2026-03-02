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
		fmt.Printf("asset_name=%s\n", release.AssetName)
		fmt.Printf("version=%s\n", release.Version())
		fmt.Printf("os=%s\n", release.OS)
		fmt.Printf("arch=%s\n", release.Arch)
		fmt.Printf("url=%s\n", release.AssetURL)
		fmt.Printf("size=%d\n", release.AssetByteSize)
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
		release, found, err := updater.DetectVersion(ctx, repo, targetVersion)
		if err != nil {
			return nil, fmt.Errorf("detect version %s: %w", targetVersion, err)
		}
		if !found {
			return nil, fmt.Errorf("version %s not found", targetVersion)
		}
		return release, nil
	}

	release, found, err := updater.DetectLatest(ctx, repo)
	if err != nil {
		return nil, fmt.Errorf("detect latest version: %w", err)
	}
	if !found {
		return nil, fmt.Errorf("no release found")
	}
	return release, nil
}
