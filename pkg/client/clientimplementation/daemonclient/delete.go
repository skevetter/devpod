package daemonclient

import (
	"context"
	"fmt"
	"time"

	clientpkg "github.com/skevetter/devpod/pkg/client"
	"github.com/skevetter/devpod/pkg/client/clientimplementation"
	"github.com/skevetter/devpod/pkg/platform"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

func (c *client) Delete(ctx context.Context, opt clientpkg.DeleteOptions) error {
	c.m.Lock()
	defer c.m.Unlock()

	baseClient, err := c.initPlatformClient(ctx)
	if err != nil {
		return err
	}

	workspace, err := platform.FindInstance(ctx, baseClient, c.workspace.UID)
	if err != nil {
		return err
	} else if workspace == nil {
		// delete the workspace folder
		err = clientimplementation.DeleteWorkspaceFolder(clientimplementation.DeleteWorkspaceFolderParams{
			Context:              c.workspace.Context,
			WorkspaceID:          c.workspace.ID,
			SSHConfigPath:        c.workspace.SSHConfigPath,
			SSHConfigIncludePath: c.workspace.SSHConfigIncludePath,
		}, c.log)
		if err != nil {
			return err
		}

		return nil
	}

	managementClient, err := baseClient.Management()
	if err != nil {
		return err
	}
	var gracePeriod *time.Duration
	if opt.GracePeriod != "" {
		duration, err := time.ParseDuration(opt.GracePeriod)
		if err == nil {
			gracePeriod = &duration
		}
	}

	// kill the command after the grace period
	if gracePeriod != nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, *gracePeriod)
		defer cancel()
	}

	// delete the workspace
	err = managementClient.Loft().ManagementV1().DevPodWorkspaceInstances(workspace.Namespace).Delete(ctx, workspace.Name, metav1.DeleteOptions{})
	if err != nil {
		if !opt.Force {
			return fmt.Errorf("delete workspace %w", err)
		}

		if !kerrors.IsNotFound(err) {
			c.log.Errorf("Error deleting workspace: %v", err)
		}
	}

	// delete the workspace folder
	err = clientimplementation.DeleteWorkspaceFolder(clientimplementation.DeleteWorkspaceFolderParams{
		Context:              c.workspace.Context,
		WorkspaceID:          c.workspace.ID,
		SSHConfigPath:        c.workspace.SSHConfigPath,
		SSHConfigIncludePath: c.workspace.SSHConfigIncludePath,
	}, c.log)
	if err != nil {
		return err
	}

	// calculate wait timeout
	waitTimeout := time.Minute
	if gracePeriod != nil {
		waitTimeout = *gracePeriod
	}

	// wait until the workspace is deleted
	c.log.Debugf("Waiting for workspace to get deleted...")
	err = wait.PollUntilContextTimeout(ctx, time.Second, waitTimeout, false, func(ctx context.Context) (done bool, err error) {
		workspaceInstance, err := managementClient.Loft().ManagementV1().DevPodWorkspaceInstances(workspace.Namespace).Get(ctx, workspace.Name, metav1.GetOptions{})
		if kerrors.IsNotFound(err) {
			return true, nil
		} else if err != nil {
			return false, fmt.Errorf("error getting workspace %w", err)
		} else if workspaceInstance.DeletionTimestamp == nil {
			// this can occur if the workspace is already deleted and was recreated
			return true, nil
		}

		c.log.Debugf("Workspace is not deleted yet, waiting again...")
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("error waiting for workspace to get deleted %w", err)
	}

	return nil
}
