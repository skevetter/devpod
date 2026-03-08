package workspace

import (
	"context"
	"fmt"

	client2 "github.com/skevetter/devpod/pkg/client"
	"github.com/skevetter/devpod/pkg/client/clientimplementation"
	"github.com/skevetter/devpod/pkg/config"
	"github.com/skevetter/devpod/pkg/platform"
	"github.com/skevetter/log"
)

// DeleteOptions holds the parameters for deleting a workspace.
type DeleteOptions struct {
	DevPodConfig   *config.Config
	Args           []string
	IgnoreNotFound bool
	Force          bool
	ClientDelete   client2.DeleteOptions
	Owner          platform.OwnerFilter
	Log            log.Logger
}

// Delete deletes a workspace, handling imported workspaces, single-machine
// cleanup, and force-deletion of broken workspaces.
func Delete(ctx context.Context, opts DeleteOptions) (string, error) {
	client, err := Get(ctx, GetOptions{
		DevPodConfig: opts.DevPodConfig,
		Args:         opts.Args,
		Owner:        opts.Owner,
		Log:          opts.Log,
	})
	if err != nil {
		return handleDeleteLoadError(ctx, opts, err)
	}

	if id, done, err := deleteImportedWorkspace(client, opts); done {
		return id, err
	}

	unlock, err := checkBeforeDelete(ctx, client, opts)
	if err != nil {
		return "", err
	}
	defer unlock()

	return deleteWorkspace(ctx, client, opts)
}

// checkBeforeDelete acquires the lock and verifies the workspace exists
// unless force-deletion is requested. It returns an unlock function that
// must be called by the caller (typically deferred) to release the lock.
func checkBeforeDelete(
	ctx context.Context,
	client client2.BaseWorkspaceClient,
	opts DeleteOptions,
) (func(), error) {
	force := opts.Force || opts.ClientDelete.Force
	if force {
		return func() {}, nil
	}

	unlock, err := lockIfNeeded(ctx, client, opts)
	if err != nil {
		return nil, err
	}

	status, err := client.Status(ctx, client2.StatusOptions{})
	if err != nil {
		unlock()
		return nil, err
	}

	ignoreNotFound := opts.IgnoreNotFound || opts.ClientDelete.IgnoreNotFound
	if status == client2.StatusNotFound && !ignoreNotFound {
		unlock()
		return nil, fmt.Errorf(
			"workspace not found, use --force to delete anyway",
		)
	}

	return unlock, nil
}

// lockIfNeeded acquires the workspace lock when not running on a platform and
// returns a function that releases it. When the platform is enabled the
// returned function is a no-op.
func lockIfNeeded(
	ctx context.Context,
	client client2.BaseWorkspaceClient,
	opts DeleteOptions,
) (func(), error) {
	if opts.ClientDelete.Platform.Enabled {
		return func() {}, nil
	}

	if err := client.Lock(ctx); err != nil {
		return nil, err
	}

	return client.Unlock, nil
}

// handleDeleteLoadError handles the case where the workspace client could not
// be loaded. It either force-deletes the folder or returns the original error.
func handleDeleteLoadError(
	ctx context.Context,
	opts DeleteOptions,
	loadErr error,
) (string, error) {
	if len(opts.Args) == 0 {
		return "", fmt.Errorf(
			"failed to load workspace: %w, "+
				"specify the workspace id to delete, e.g. 'devpod delete my-workspace --force'",
			loadErr,
		)
	}

	workspaceID := Exists(ctx, opts.DevPodConfig, opts.Args, "", opts.Owner, opts.Log)
	if workspaceID == "" {
		if opts.IgnoreNotFound {
			return "", nil
		}

		return "", fmt.Errorf("workspace %s not found", opts.Args[0])
	}

	if !opts.Force {
		opts.Log.Errorf(
			"failed to load workspace, use --force to delete anyway",
		)

		return "", loadErr
	}

	return forceDeleteFolder(opts, workspaceID)
}

// forceDeleteFolder removes the workspace folder when the workspace client
// cannot be loaded and --force is set.
func forceDeleteFolder(opts DeleteOptions, workspaceID string) (string, error) {
	opts.Log.Errorf("error retrieving workspace, force-deleting folder")

	err := clientimplementation.DeleteWorkspaceFolder(
		clientimplementation.DeleteWorkspaceFolderParams{
			Context:     opts.DevPodConfig.DefaultContext,
			WorkspaceID: workspaceID,
		},
		opts.Log,
	)
	if err != nil {
		return "", err
	}

	opts.Log.Donef("deleted workspace %s", workspaceID)

	return workspaceID, nil
}

// deleteImportedWorkspace removes only the local folder for imported
// workspaces when --force is not set. The bool return indicates whether
// the delete was handled (caller should return).
func deleteImportedWorkspace(
	client client2.BaseWorkspaceClient,
	opts DeleteOptions,
) (string, bool, error) {
	wsCfg := client.WorkspaceConfig()
	if opts.Force || !wsCfg.Imported {
		return "", false, nil
	}

	err := clientimplementation.DeleteWorkspaceFolder(
		clientimplementation.DeleteWorkspaceFolderParams{
			Context:              opts.DevPodConfig.DefaultContext,
			WorkspaceID:          client.Workspace(),
			SSHConfigPath:        wsCfg.SSHConfigPath,
			SSHConfigIncludePath: wsCfg.SSHConfigIncludePath,
		},
		opts.Log,
	)
	if err != nil {
		return "", true, err
	}

	opts.Log.Donef(
		"skipped remote deletion of workspace %s, use --force to delete remotely",
		client.Workspace(),
	)

	return client.Workspace(), true, nil
}

// deleteWorkspace handles single-machine cleanup and the actual workspace
// deletion.
func deleteWorkspace(
	ctx context.Context,
	client client2.BaseWorkspaceClient,
	opts DeleteOptions,
) (string, error) {
	wasDeleted, err := deleteSingleMachine(ctx, client, opts)
	if err != nil {
		return "", err
	}
	if wasDeleted {
		return client.Workspace(), nil
	}

	if err := client.Delete(ctx, opts.ClientDelete); err != nil {
		return "", err
	}

	return client.Workspace(), nil
}

// deleteSingleMachine deletes the underlying machine when this is the last
// workspace using it in single-machine mode.
func deleteSingleMachine(
	ctx context.Context,
	client client2.BaseWorkspaceClient,
	opts DeleteOptions,
) (bool, error) {
	singleMachineName := SingleMachineName(opts.DevPodConfig, client.Provider(), opts.Log)
	if !opts.DevPodConfig.Current().IsSingleMachine(client.Provider()) ||
		client.WorkspaceConfig().Machine.ID != singleMachineName {
		return false, nil
	}

	otherExists, err := hasOtherWorkspaces(ctx, client, singleMachineName, opts)
	if err != nil {
		return false, fmt.Errorf("list workspaces: %w", err)
	}
	if otherExists {
		return false, nil
	}

	machineClient, err := GetMachine(opts.DevPodConfig, []string{singleMachineName}, opts.Log)
	if err != nil {
		return false, fmt.Errorf("get machine: %w", err)
	}

	if err := machineClient.Delete(ctx, opts.ClientDelete); err != nil {
		return false, fmt.Errorf("delete machine: %w", err)
	}

	wsCfg := client.WorkspaceConfig()
	err = clientimplementation.DeleteWorkspaceFolder(
		clientimplementation.DeleteWorkspaceFolderParams{
			Context:              client.Context(),
			WorkspaceID:          client.Workspace(),
			SSHConfigPath:        wsCfg.SSHConfigPath,
			SSHConfigIncludePath: wsCfg.SSHConfigIncludePath,
		},
		opts.Log,
	)
	if err != nil {
		return false, err
	}

	opts.Log.Donef("deleted workspace %s", client.Workspace())

	return true, nil
}

// hasOtherWorkspaces reports whether any other workspace shares the same
// single-machine.
func hasOtherWorkspaces(
	ctx context.Context,
	client client2.BaseWorkspaceClient,
	machineName string,
	opts DeleteOptions,
) (bool, error) {
	workspaces, err := List(ctx, opts.DevPodConfig, false, opts.Owner, opts.Log)
	if err != nil {
		return false, err
	}

	for _, ws := range workspaces {
		if ws.ID != client.Workspace() && ws.Machine.ID == machineName {
			return true, nil
		}
	}

	return false, nil
}
