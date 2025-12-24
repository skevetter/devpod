package workspace

import (
	"context"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/cmd/flags"
	"github.com/skevetter/devpod/pkg/agent"
	provider2 "github.com/skevetter/devpod/pkg/provider"
	"github.com/skevetter/log"
	"github.com/spf13/cobra"
)

// BuildCmd holds the cmd flags
type BuildCmd struct {
	*flags.GlobalFlags

	WorkspaceInfo string
}

// NewBuildCmd creates a new command
func NewBuildCmd(flags *flags.GlobalFlags) *cobra.Command {
	cmd := &BuildCmd{
		GlobalFlags: flags,
	}
	buildCmd := &cobra.Command{
		Use:   "build",
		Short: "Builds a devcontainer",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return cmd.Run(context.Background())
		},
	}
	buildCmd.Flags().StringVar(&cmd.WorkspaceInfo, "workspace-info", "", "The workspace info")
	_ = buildCmd.MarkFlagRequired("workspace-info")
	return buildCmd
}

// Run runs the command logic
func (cmd *BuildCmd) Run(ctx context.Context) error {
	// write workspace info
	shouldExit, workspaceInfo, err := agent.WriteWorkspaceInfoAndDeleteOld(cmd.WorkspaceInfo, func(workspaceInfo *provider2.AgentWorkspaceInfo, log log.Logger) error {
		return deleteWorkspace(ctx, workspaceInfo, log)
	}, log.Default.ErrorStreamOnly())
	if err != nil {
		return err
	} else if shouldExit {
		return nil
	}

	// make sure daemon does shut us down while we are doing things
	agent.CreateWorkspaceBusyFile(workspaceInfo.Origin)
	defer agent.DeleteWorkspaceBusyFile(workspaceInfo.Origin)

	// initialize the workspace
	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	_, logger, credentialsDir, err := initWorkspace(cancelCtx, cancel, workspaceInfo, cmd.Debug, false)
	if err != nil {
		return err
	} else if credentialsDir != "" {
		defer func() {
			_ = os.RemoveAll(credentialsDir)
		}()
	}

	runner, err := CreateRunner(workspaceInfo, logger)
	if err != nil {
		return err
	}

	// if there is no platform specified, we use empty to let
	// the builder find out itself.
	platforms := workspaceInfo.CLIOptions.Platforms
	if len(platforms) == 0 {
		platforms = []string{""}
	}

	// build and push images
	for _, platform := range platforms {
		// build the image
		imageName, err := runner.Build(ctx, provider2.BuildOptions{
			CLIOptions:    workspaceInfo.CLIOptions,
			RegistryCache: workspaceInfo.RegistryCache,
			Platform:      platform,
			ExportCache:   true,
		})
		if err != nil {
			logger.Errorf("Error building image: %v", err)
			return fmt.Errorf("build %w", err)
		}

		if workspaceInfo.CLIOptions.SkipPush {
			logger.WithFields(logrus.Fields{
				"imageName": imageName,
			})
			logger.Donef("done building image")
		} else {
			logger.WithFields(logrus.Fields{
				"imageName": imageName,
			})
			logger.Donef("done building and pushing image")
		}
	}

	return nil
}

func deleteWorkspace(ctx context.Context, workspaceInfo *provider2.AgentWorkspaceInfo, log log.Logger) error {
	err := removeContainer(ctx, workspaceInfo, log)
	if err != nil {
		log.Errorf("Removing container: %v", err)
	}

	_ = os.RemoveAll(workspaceInfo.Origin)
	return nil
}
