package cmd

import (
	"fmt"

	"github.com/skevetter/devpod/pkg/upgrade"
	"github.com/skevetter/log"
	"github.com/spf13/cobra"
)

// UpgradeCmd is a struct that defines a command call for "upgrade".
type UpgradeCmd struct {
	log     log.Logger
	Version string
	DryRun  bool
}

// NewUpgradeCmd creates a new upgrade command.
func NewUpgradeCmd() *cobra.Command {
	cmd := &UpgradeCmd{log: log.GetInstance()}
	upgradeCmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade the DevPod CLI to the newest version",
		Args:  cobra.NoArgs,
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			ctx := cobraCmd.Context()
			if err := upgrade.Upgrade(ctx, cmd.Version, cmd.DryRun, cmd.log); err != nil {
				return fmt.Errorf("unable to upgrade: %w", err)
			}
			return nil
		},
	}

	upgradeCmd.Flags().
		StringVar(&cmd.Version, "version", "",
			"The version to update to. Defaults to the latest stable version available")
	upgradeCmd.Flags().
		BoolVar(&cmd.DryRun, "dry-run", false, "Show which version would be downloaded without actually upgrading")
	return upgradeCmd
}
