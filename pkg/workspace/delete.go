package workspace

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	client2 "github.com/skevetter/devpod/pkg/client"
	"github.com/skevetter/devpod/pkg/client/clientimplementation"
	"github.com/skevetter/devpod/pkg/config"
	"github.com/skevetter/devpod/pkg/platform"
	"github.com/skevetter/log"
)

func Delete(ctx context.Context, devPodConfig *config.Config, args []string, ignoreNotFound, force bool, deleteOptions client2.DeleteOptions, owner platform.OwnerFilter, log log.Logger) (string, error) {
	// try to load workspace
	client, err := Get(ctx, devPodConfig, args, false, owner, false, log)
	if err != nil {
		if len(args) == 0 {
			return "", fmt.Errorf("cannot delete workspace because there was an error loading the workspace: %w. Please specify the id of the workspace you want to delete. E.g. 'devpod delete my-workspace --force'", err)
		}

		workspaceID := Exists(ctx, devPodConfig, args, "", owner, log)
		if workspaceID == "" {
			if ignoreNotFound {
				return "", nil
			}

			return "", fmt.Errorf("couldn't find workspace %s", args[0])
		} else if !force {
			log.Errorf("cannot delete workspace because there was an error loading the workspace. Run with --force to ignore this error")
			return "", err
		}

		// print error
		log.Errorf("Error retrieving workspace: %v", err)

		// delete workspace folder
		err = clientimplementation.DeleteWorkspaceFolder(clientimplementation.DeleteWorkspaceFolderParams{
			Context:              devPodConfig.DefaultContext,
			WorkspaceID:          workspaceID,
			SSHConfigPath:        "",
			SSHConfigIncludePath: "",
		}, log)
		if err != nil {
			return "", err
		}

		log.WithFields(logrus.Fields{
			"workspace": workspaceID,
		})
		log.Donef("deleted workspace")
		return workspaceID, nil
	}

	// only remove local folder if workspace is imported or pro
	workspaceConfig := client.WorkspaceConfig()
	if !force && workspaceConfig.Imported {
		// delete workspace folder
		err = clientimplementation.DeleteWorkspaceFolder(clientimplementation.DeleteWorkspaceFolderParams{
			Context:              devPodConfig.DefaultContext,
			WorkspaceID:          client.Workspace(),
			SSHConfigPath:        workspaceConfig.SSHConfigPath,
			SSHConfigIncludePath: workspaceConfig.SSHConfigIncludePath,
		}, log)
		if err != nil {
			return "", err
		}

		log.Donef("Skip remote deletion of workspace %s, if you really want to delete this workspace also remotely, run with --force", client.Workspace())
		return client.Workspace(), nil
	}

	// get instance status
	if !force {
		// lock workspace only if we don't force deletion
		if !deleteOptions.Platform.Enabled {
			err := client.Lock(ctx)
			if err != nil {
				return "", err
			}
			defer client.Unlock()
		}

		// retrieve instance status
		instanceStatus, err := client.Status(ctx, client2.StatusOptions{})
		if err != nil {
			return "", err
		} else if instanceStatus == client2.StatusNotFound {
			return "", fmt.Errorf("cannot delete workspace because it couldn't be found. Run with --force to ignore this error")
		}
	}

	// delete if single machine provider
	wasDeleted, err := deleteSingleMachine(ctx, client, devPodConfig, deleteOptions, log)
	if err != nil {
		return "", err
	} else if wasDeleted {
		return client.Workspace(), nil
	}

	// destroy environment
	err = client.Delete(ctx, deleteOptions)
	if err != nil {
		return "", err
	}

	return client.Workspace(), nil
}

func deleteSingleMachine(ctx context.Context, client client2.BaseWorkspaceClient, devPodConfig *config.Config, deleteOptions client2.DeleteOptions, log log.Logger) (bool, error) {
	// check if single machine
	singleMachineName := SingleMachineName(devPodConfig, client.Provider(), log)
	if !devPodConfig.Current().IsSingleMachine(client.Provider()) || client.WorkspaceConfig().Machine.ID != singleMachineName {
		return false, nil
	}

	// try to find other workspace with same machine
	workspaces, err := List(ctx, devPodConfig, false, platform.SelfOwnerFilter, log)
	if err != nil {
		return false, fmt.Errorf("list workspaces %w", err)
	}

	// loop workspaces
	foundOther := false
	for _, workspace := range workspaces {
		if workspace.ID == client.Workspace() || workspace.Machine.ID != singleMachineName {
			continue
		}

		foundOther = true
		break
	}
	if foundOther {
		return false, nil
	}

	// if we haven't found another workspace on this machine, delete the whole machine
	machineClient, err := GetMachine(devPodConfig, []string{singleMachineName}, log)
	if err != nil {
		return false, fmt.Errorf("get machine %w", err)
	}

	// delete the machine
	err = machineClient.Delete(ctx, deleteOptions)
	if err != nil {
		return false, fmt.Errorf("delete machine %w", err)
	}

	// delete workspace folder
	err = clientimplementation.DeleteWorkspaceFolder(clientimplementation.DeleteWorkspaceFolderParams{
		Context:              client.Context(),
		WorkspaceID:          client.Workspace(),
		SSHConfigPath:        client.WorkspaceConfig().SSHConfigPath,
		SSHConfigIncludePath: client.WorkspaceConfig().SSHConfigIncludePath,
	}, log)
	if err != nil {
		return false, err
	}

	log.WithFields(logrus.Fields{
		"workspace": client.Workspace(),
	})
	log.Donef("deleted workspace")
	return true, nil
}
