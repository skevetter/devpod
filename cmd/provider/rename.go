package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/skevetter/devpod/cmd/flags"
	"github.com/skevetter/devpod/pkg/config"
	"github.com/skevetter/devpod/pkg/provider"
	workspace "github.com/skevetter/devpod/pkg/workspace"
	"github.com/skevetter/log"
	"github.com/spf13/cobra"
)

// RenameCmd holds the cmd flags.
type RenameCmd struct {
	*flags.GlobalFlags
}

// NewRenameCmd creates a new command.
func NewRenameCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &RenameCmd{
		GlobalFlags: globalFlags,
	}

	return &cobra.Command{
		Use:   "rename",
		Short: "Rename a provider",
		Long: `Renames a provider by cloning it with the new name, automatically rebinds all workspaces
that are bound to it to use the new provider name, and cleans up the old provider.

Example:
  devpod provider rename my-provider my-new-provider
`,
		Args: cobra.ExactArgs(2),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			return cmd.Run(cobraCmd, args)
		},
	}
}

// getWorkspacesToRebind gathers workspaces that need to be rebound from provider name.
func getWorkspacesToRebind(devPodConfig *config.Config, name string) ([]*provider.Workspace, error) {
	workspaces, err := workspace.ListLocalWorkspaces(devPodConfig.DefaultContext, false, log.Default)
	if err != nil {
		return nil, fmt.Errorf("listing workspaces: %w", err)
	}
	var workspacesToRebind []*provider.Workspace
	for _, ws := range workspaces {
		if ws.Provider.Name == name {
			workspacesToRebind = append(workspacesToRebind, ws)
		}
	}

	if len(workspacesToRebind) > 0 {
		log.Default.Infof("rebinding %d workspace(s) from provider '%s'", len(workspacesToRebind), name)
	} else {
		log.Default.Info("no workspaces found that are bound to this provider")
	}

	return workspacesToRebind, nil
}

// rebindWorkspaces updates the provider name for the given workspaces and saves the configurations.
func rebindWorkspaces(
	devPodConfig *config.Config,
	workspacesToRebind []*provider.Workspace,
	newName string,
) ([]*provider.Workspace, error) {
	var aggregateError error
	var successfulRebinds []*provider.Workspace

	for _, ws := range workspacesToRebind {
		log.Default.Infof("rebinding workspace %s to provider %s", ws.ID, newName)

		err := workspace.SwitchProvider(context.Background(), devPodConfig, ws, newName)
		if err != nil {
			log.Default.Errorf("failed to rebind workspace %s: %v", ws.ID, err)
			aggregateError = errors.Join(aggregateError, err)
		} else {
			successfulRebinds = append(successfulRebinds, ws)
		}
	}
	return successfulRebinds, aggregateError
}

// checks if default provider is touched by rename and updates default provider to newName.
func adjustDefaultProvider(devPodConfig *config.Config, oldName string, newName string) error {
	if devPodConfig.Current().DefaultProvider == oldName {
		devPodConfig.Current().DefaultProvider = newName
		err := config.SaveConfig(devPodConfig)
		if err != nil {
			devPodConfig.Current().DefaultProvider = oldName
			log.Default.Errorf("failed to update default provider to %s: %v", newName, err)
			return err
		} else {
			log.Default.Infof("updated default provider from %s to %s", oldName, newName)
		}
	}
	return nil
}

func rollback(
	devPodConfig *config.Config,
	workspacesTouched []*provider.Workspace,
	oldName string,
	newName string,
) error {
	log.Default.Info("rolling back changes")
	var _, err = rebindWorkspaces(devPodConfig, workspacesTouched, oldName)
	if err != nil {
		return err
	}

	err = DeleteProviderConfig(devPodConfig, newName, true)
	if err == nil {
		log.Default.Infof("cloned provider %s deleted successfully", newName)
	}

	return err
}

// validateProviderName validates the new provider name.
func validateProviderName(newName string) error {
	if strings.TrimSpace(newName) == "" {
		return fmt.Errorf("provider name cannot be empty")
	}
	if provider.ProviderNameRegEx.MatchString(newName) {
		return fmt.Errorf("provider name can only include lowercase letters, numbers or dashes")
	}
	if len(newName) > 32 {
		return fmt.Errorf("provider name cannot be longer than 32 characters")
	}
	return nil
}

// cloneAndRebindProvider handles the core logic of cloning and rebinding workspaces.
func cloneAndRebindProvider(
	devPodConfig *config.Config,
	oldName,
	newName string,
	workspacesToRebind []*provider.Workspace) ([]*provider.Workspace, error) {
	log.Default.Info("renaming provider using clone and rebinding workspaces")

	_, cloneErr := workspace.CloneProvider(devPodConfig, newName, oldName, log.Default)
	if cloneErr != nil {
		return nil, fmt.Errorf("failed to clone provider: %w", cloneErr)
	}

	log.Default.Infof("provider successfully cloned from %s to %s", oldName, newName)

	successfulRebinds, renameErr := rebindWorkspaces(devPodConfig, workspacesToRebind, newName)

	if renameErr == nil {
		renameErr = adjustDefaultProvider(devPodConfig, oldName, newName)
	}

	return successfulRebinds, renameErr
}

// cleanupOldProvider deletes the old provider after successful rename.
func cleanupOldProvider(devPodConfig *config.Config, oldName, newName string) error {
	deleteErr := DeleteProviderConfig(devPodConfig, oldName, true)
	if deleteErr != nil {
		log.Default.Errorf("failed to delete old provider %s: %v", oldName, deleteErr)
		return fmt.Errorf("failed to delete old provider after successful rename: %w", deleteErr)
	}

	log.Default.Infof("old provider %s deleted successfully", oldName)

	_, err := workspace.FindProvider(devPodConfig, newName, log.Default)
	if err != nil {
		return fmt.Errorf("failed to load renamed provider %s: %w", newName, err)
	}

	return nil
}

// Run executes the command.
func (cmd *RenameCmd) Run(cobraCmd *cobra.Command, args []string) error {
	oldName := args[0]
	newName := args[1]

	if err := validateProviderName(newName); err != nil {
		return err
	}

	devPodConfig, err := config.LoadConfig(cmd.Context, cmd.Provider)
	if err != nil {
		return err
	}

	workspacesToRebind, err := getWorkspacesToRebind(devPodConfig, oldName)
	if err != nil {
		return err
	}

	_, newProviderExists := devPodConfig.Current().Providers[newName]
	if newProviderExists {
		return fmt.Errorf("provider %s already exists", newName)
	}

	successfulRebinds, renameErr := cloneAndRebindProvider(devPodConfig, oldName, newName, workspacesToRebind)

	if renameErr != nil {
		log.Default.Errorf("failed to rename provider %s to %s: %v", oldName, newName, renameErr)
		err = rollback(devPodConfig, successfulRebinds, oldName, newName)
		return errors.Join(renameErr, err)
	}

	err = cleanupOldProvider(devPodConfig, oldName, newName)
	if err != nil {
		return err
	}

	log.Default.Donef("successfully renamed provider %s to %s and rebound all associated workspaces", oldName, newName)
	return nil
}
