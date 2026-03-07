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

// RenameCmd implements the provider rename command.
type RenameCmd struct {
	*flags.GlobalFlags
}

// NewRenameCmd creates a new command for renaming a provider.
func NewRenameCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &RenameCmd{
		GlobalFlags: globalFlags,
	}

	return &cobra.Command{
		Use:   "rename <current-name> <new-name>",
		Short: "Rename a provider",
		Args:  cobra.ExactArgs(2),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			return cmd.Run(cobraCmd.Context(), args)
		},
	}
}

// Run validates inputs, loads config, and executes the provider rename.
func (cmd *RenameCmd) Run(ctx context.Context, args []string) error {
	oldName, newName := args[0], args[1]

	if oldName == newName {
		return fmt.Errorf("new name is the same as the current name")
	}

	if err := validateProviderName(newName); err != nil {
		return err
	}

	devPodConfig, err := config.LoadConfig(cmd.Context, cmd.Provider)
	if err != nil {
		return err
	}

	if err := validateProviderRename(devPodConfig, oldName); err != nil {
		return err
	}

	if devPodConfig.Current().Providers[newName] != nil {
		return fmt.Errorf("provider %s already exists", newName)
	}

	return renameProvider(ctx, devPodConfig, oldName, newName)
}

// validateProviderName checks that the given name is non-empty, matches the
// allowed character set (lowercase letters, numbers, dashes), and does not
// exceed the maximum length of 32 characters.
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

// getWorkspacesForProvider returns all local workspaces whose provider matches
// the given name.
func getWorkspacesForProvider(
	devPodConfig *config.Config,
	providerName string,
) ([]*provider.Workspace, error) {
	workspaces, err := workspace.ListLocalWorkspaces(
		devPodConfig.DefaultContext,
		false,
		log.Default,
	)
	if err != nil {
		return nil, fmt.Errorf("listing workspaces: %w", err)
	}
	var matched []*provider.Workspace
	for _, ws := range workspaces {
		if ws.Provider.Name == providerName {
			matched = append(matched, ws)
		}
	}
	return matched, nil
}

// getMachinesForProvider returns all machines whose provider matches the given
// name.
func getMachinesForProvider(
	devPodConfig *config.Config,
	providerName string,
) ([]*provider.Machine, error) {
	machines, err := workspace.ListMachines(devPodConfig, log.Default)
	if err != nil {
		return nil, fmt.Errorf("listing machines: %w", err)
	}
	var matched []*provider.Machine
	for _, m := range machines {
		if m.Provider.Name == providerName {
			matched = append(matched, m)
		}
	}
	return matched, nil
}

// switchWorkspaces updates each workspace to reference the new provider name.
// It stops on the first failure and returns the successfully switched
// workspaces so the caller can roll them back.
func switchWorkspaces(
	ctx context.Context,
	devPodConfig *config.Config,
	workspaces []*provider.Workspace,
	newName string,
) ([]*provider.Workspace, error) {
	var switched []*provider.Workspace
	for _, ws := range workspaces {
		if err := workspace.SwitchProvider(ctx, devPodConfig, ws, newName); err != nil {
			return switched, fmt.Errorf("failed to switch workspace %s: %w", ws.ID, err)
		}
		switched = append(switched, ws)
	}
	return switched, nil
}

// switchMachines updates each machine to reference the new provider name.
// It stops on the first failure and returns the successfully switched
// machines so the caller can roll them back.
func switchMachines(machines []*provider.Machine, newName string) ([]*provider.Machine, error) {
	var switched []*provider.Machine
	for _, m := range machines {
		oldName := m.Provider.Name
		m.Provider.Name = newName
		if err := provider.SaveMachineConfig(m); err != nil {
			m.Provider.Name = oldName
			return switched, fmt.Errorf("failed to switch machine %s: %w", m.ID, err)
		}
		switched = append(switched, m)
	}
	return switched, nil
}

// setDefaultProvider updates the default provider setting if it currently
// points to oldName. Returns true if the default was changed.
func setDefaultProvider(devPodConfig *config.Config, oldName, newName string) (bool, error) {
	if devPodConfig.Current().DefaultProvider != oldName {
		return false, nil
	}
	devPodConfig.Current().DefaultProvider = newName
	if err := config.SaveConfig(devPodConfig); err != nil {
		devPodConfig.Current().DefaultProvider = oldName
		return false, err
	}
	return true, nil
}

// renameState tracks the mutations performed during a rename so they can be
// undone if a later step fails.
type renameState struct {
	devPodConfig       *config.Config
	switchedWorkspaces []*provider.Workspace
	switchedMachines   []*provider.Machine
	defaultChanged     bool
	oldName, newName   string
}

// restoreProviderState reverts all recorded mutations in reverse order: default provider,
// workspaces, machines, and finally the provider directory move.
func (r *renameState) restoreProviderState(ctx context.Context) error {
	log.Default.Info("rolling back changes")
	var errs error

	if r.defaultChanged {
		r.devPodConfig.Current().DefaultProvider = r.oldName
		if err := config.SaveConfig(r.devPodConfig); err != nil {
			errs = errors.Join(errs, fmt.Errorf("rollback default provider: %w", err))
		}
	}

	_, err := switchWorkspaces(ctx, r.devPodConfig, r.switchedWorkspaces, r.oldName)
	errs = errors.Join(errs, err)

	_, err = switchMachines(r.switchedMachines, r.oldName)
	errs = errors.Join(errs, err)

	if moveErr := workspace.MoveProvider(r.devPodConfig, r.newName, r.oldName); moveErr != nil {
		errs = errors.Join(errs, fmt.Errorf("rollback move provider: %w", moveErr))
	}

	return errs
}

// validateProviderRename verifies that the provider exists, is not a pro
// provider, is not backing a pro instance, and has configuration state.
func validateProviderRename(devPodConfig *config.Config, oldName string) error {
	providerWithOptions, err := workspace.FindProvider(devPodConfig, oldName, log.Default)
	if err != nil {
		return fmt.Errorf("provider %s not found", oldName)
	}

	if providerWithOptions.Config.IsProxyProvider() ||
		providerWithOptions.Config.IsDaemonProvider() {
		return fmt.Errorf("cannot rename a pro provider; pro providers are managed by the platform")
	}

	proInstances, err := workspace.ListProInstances(devPodConfig, log.Default)
	if err != nil {
		return fmt.Errorf("listing pro instances: %w", err)
	}
	for _, inst := range proInstances {
		if inst.Provider == oldName {
			return fmt.Errorf(
				"cannot rename provider %s: it is used by pro instance %s",
				oldName,
				inst.Host,
			)
		}
	}

	if devPodConfig.Current().Providers[oldName] == nil {
		return fmt.Errorf("provider %s has no configuration state", oldName)
	}

	return nil
}

// renameProvider performs the rename: moves the provider directory, switches all
// associated workspaces and machines, and adjusts the default provider. If any
// step fails the entire operation is rolled back.
func renameProvider(
	ctx context.Context,
	devPodConfig *config.Config,
	oldName, newName string,
) error {
	workspaces, err := getWorkspacesForProvider(devPodConfig, oldName)
	if err != nil {
		return err
	}

	machines, err := getMachinesForProvider(devPodConfig, oldName)
	if err != nil {
		return err
	}

	if err := workspace.MoveProvider(devPodConfig, oldName, newName); err != nil {
		return fmt.Errorf("moving provider: %w", err)
	}

	rb := &renameState{devPodConfig: devPodConfig, oldName: oldName, newName: newName}

	rb.switchedWorkspaces, err = switchWorkspaces(ctx, devPodConfig, workspaces, newName)
	if err != nil {
		return errors.Join(err, rb.restoreProviderState(ctx))
	}

	rb.switchedMachines, err = switchMachines(machines, newName)
	if err != nil {
		return errors.Join(err, rb.restoreProviderState(ctx))
	}

	rb.defaultChanged, err = setDefaultProvider(devPodConfig, oldName, newName)
	if err != nil {
		return errors.Join(err, rb.restoreProviderState(ctx))
	}

	log.Default.Donef("renamed provider %s to %s", oldName, newName)
	return nil
}
