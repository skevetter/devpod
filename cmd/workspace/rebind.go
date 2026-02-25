package workspace

import (
	"context"
	"fmt"

	"github.com/skevetter/devpod/cmd/flags"
	"github.com/skevetter/devpod/pkg/config"
	providerpkg "github.com/skevetter/devpod/pkg/provider"
	"github.com/skevetter/devpod/pkg/workspace"
	"github.com/skevetter/log"
	"github.com/spf13/cobra"
)

// RebindCmd holds the cmd flags.
type RebindCmd struct {
	*flags.GlobalFlags
}

// NewRebindCmd creates a new command.
func NewRebindCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &RebindCmd{
		GlobalFlags: globalFlags,
	}

	return &cobra.Command{
		Use:   "rebind <workspace-name> <new-provider-name>",
		Short: "Rebinds a workspace to a new provider",
		Args:  cobra.ExactArgs(2),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			return cmd.Run(args)
		},
	}
}

// Run executes the command.
func (cmd *RebindCmd) Run(args []string) error {
	workspaceName := args[0]
	newProviderName := args[1]

	devPodConfig, err := config.LoadConfig(cmd.Context, cmd.Provider)
	if err != nil {
		return err
	}

	workspaceID := workspace.ToID(workspaceName)

	workspaceConfig, err := providerpkg.LoadWorkspaceConfig(devPodConfig.DefaultContext, workspaceID)
	if err != nil {
		return fmt.Errorf("loading workspace config: %w", err)
	}

	if _, err := workspace.FindProvider(devPodConfig, newProviderName, log.Default); err != nil {
		return fmt.Errorf("provider %s does not exist: %w", newProviderName, err)
	}

	log.Default.Infof(
		"Rebinding workspace %s (ID: %s) from provider %s to %s",
		workspaceName,
		workspaceID,
		workspaceConfig.Provider.Name,
		newProviderName,
	)

	err = workspace.SwitchProvider(context.Background(), devPodConfig, workspaceConfig, newProviderName)
	if err != nil {
		return fmt.Errorf("switching provider: %w", err)
	}

	log.Default.Infof("Workspace %s (ID: %s) rebound to provider %s", workspaceName, workspaceID, newProviderName)

	return nil
}
