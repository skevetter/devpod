package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/skevetter/devpod/cmd/flags"
	"github.com/skevetter/devpod/pkg/client"
	"github.com/skevetter/devpod/pkg/client/clientimplementation"
	"github.com/skevetter/devpod/pkg/config"
	"github.com/skevetter/devpod/pkg/image"
	"github.com/skevetter/devpod/pkg/provider"
	workspace2 "github.com/skevetter/devpod/pkg/workspace"
	"github.com/skevetter/log"
	"github.com/spf13/cobra"
)

// BuildCmd holds the cmd flags
type BuildCmd struct {
	*flags.GlobalFlags
	provider.CLIOptions

	ProviderOptions []string

	SkipDelete bool
	Machine    string
}

// NewBuildCmd creates a new command
func NewBuildCmd(flags *flags.GlobalFlags) *cobra.Command {
	cmd := &BuildCmd{
		GlobalFlags: flags,
	}
	buildCmd := &cobra.Command{
		Use:   "build [flags] [workspace-path|workspace-name]",
		Short: "Builds a workspace",
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			ctx := cobraCmd.Context()
			devPodConfig, err := config.LoadConfig(cmd.Context, cmd.Provider)
			if err != nil {
				return err
			}

			// validate flags
			// PushDuringBuild and SkipPush are mutually exclusive: one enables pushing during
			// build, the other disables all pushing. Using both together is contradictory.
			if cmd.PushDuringBuild && cmd.SkipPush {
				return fmt.Errorf("cannot use --push and --skip-push together")
			}
			// PushDuringBuild requires a repository because it pushes directly to a registry.
			if cmd.PushDuringBuild && cmd.Repository == "" {
				return fmt.Errorf("--push requires --repository to be specified")
			}

			// check permissions
			if !cmd.SkipPush && cmd.Repository != "" {
				err = image.CheckPushPermissions(cmd.Repository)
				if err != nil {
					return fmt.Errorf("cannot push to %s, please make sure you have push permissions to repository %s", cmd.Repository, cmd.Repository)
				}
			}

			// validate tags
			if len(cmd.Tag) > 0 {
				if err := image.ValidateTags(cmd.Tag); err != nil {
					return fmt.Errorf("cannot build image, %w", err)
				}
			}

			if devPodConfig.ContextOption(config.ContextOptionSSHStrictHostKeyChecking) == "true" {
				cmd.StrictHostKeyChecking = true
			}

			// create a temporary workspace
			exists := workspace2.Exists(ctx, devPodConfig, args, "", cmd.Owner, log.Default)
			sshConfigFile, err := os.CreateTemp("", "devpodssh.config")
			if err != nil {
				return err
			}
			sshConfigPath := sshConfigFile.Name()
			// defer removal of temporary ssh config file
			defer func() { _ = os.Remove(sshConfigPath) }()

			baseWorkspaceClient, err := workspace2.Resolve(
				ctx,
				devPodConfig,
				workspace2.ResolveParams{
					IDE:                  "",
					IDEOptions:           nil,
					Args:                 args,
					DesiredID:            "",
					DesiredMachine:       cmd.Machine,
					ProviderUserOptions:  cmd.ProviderOptions,
					ReconfigureProvider:  false,
					DevContainerImage:    cmd.DevContainerImage,
					DevContainerPath:     cmd.DevContainerPath,
					SSHConfigPath:        sshConfigPath,
					SSHConfigIncludePath: "",
					Source:               nil,
					UID:                  cmd.UID,
					ChangeLastUsed:       false,
					Owner:                cmd.Owner,
				},
				log.Default,
			)
			if err != nil {
				return err
			}

			// delete workspace if we have created it
			if exists == "" && !cmd.SkipDelete {
				defer func() {
					err = baseWorkspaceClient.Delete(ctx, client.DeleteOptions{Force: true})
					if err != nil {
						log.Default.Errorf("Error deleting workspace: %v", err)
					}
				}()
			}

			// check if regular workspace client
			workspaceClient, ok := baseWorkspaceClient.(client.WorkspaceClient)
			if !ok {
				return fmt.Errorf("building is currently not supported for proxy providers")
			}

			return cmd.Run(ctx, workspaceClient)
		},
	}

	buildCmd.Flags().StringVar(&cmd.DevContainerImage, "devcontainer-image", "", "The container image to use, this will override the devcontainer.json value in the project")
	buildCmd.Flags().StringVar(&cmd.DevContainerPath, "devcontainer-path", "", "The path to the devcontainer.json relative to the project")
	buildCmd.Flags().StringSliceVar(&cmd.ProviderOptions, "provider-option", []string{}, "Provider option in the form KEY=VALUE")
	buildCmd.Flags().BoolVar(&cmd.SkipDelete, "skip-delete", false, "If true will not delete the workspace after building it")
	buildCmd.Flags().StringVar(&cmd.Machine, "machine", "", "The machine to use for this workspace. The machine needs to exist beforehand or the command will fail. If the workspace already exists, this option has no effect")
	buildCmd.Flags().StringVar(&cmd.Repository, "repository", "", "The repository to push to")
	buildCmd.Flags().StringSliceVar(&cmd.Tag, "tag", []string{}, "Image Tag(s) in the form of a comma separated list --tag latest,arm64 or multiple flags --tag latest --tag arm64")
	buildCmd.Flags().StringSliceVar(&cmd.Platforms, "platform", []string{}, "Set target platform for build")
	buildCmd.Flags().BoolVar(&cmd.SkipPush, "skip-push", false, "If true will not push the image to the repository, useful for testing")
	buildCmd.Flags().BoolVar(&cmd.PushDuringBuild, "push", false,
		"Push image directly to registry during build, skipping load to local daemon.",
	)
	buildCmd.Flags().Var(&cmd.GitCloneStrategy, "git-clone-strategy", "The git clone strategy DevPod uses to checkout git based workspaces. Can be full (default), blobless, treeless or shallow")
	buildCmd.Flags().BoolVar(&cmd.GitCloneRecursiveSubmodules, "git-clone-recursive-submodules", false, "If true will clone git submodule repositories recursively")

	// TESTING
	buildCmd.Flags().BoolVar(&cmd.ForceBuild, "force-build", false, "TESTING ONLY")
	buildCmd.Flags().BoolVar(&cmd.ForceInternalBuildKit, "force-internal-buildkit", false, "TESTING ONLY")
	_ = buildCmd.Flags().MarkHidden("force-build")
	_ = buildCmd.Flags().MarkHidden("force-internal-buildkit")
	return buildCmd
}

func (cmd *BuildCmd) Run(ctx context.Context, client client.WorkspaceClient) error {
	// build workspace
	err := cmd.build(ctx, client, log.Default)
	if err != nil {
		return err
	}

	return nil
}

func (cmd *BuildCmd) build(ctx context.Context, workspaceClient client.WorkspaceClient, log log.Logger) error {
	err := workspaceClient.Lock(ctx)
	if err != nil {
		return err
	}
	defer workspaceClient.Unlock()

	err = clientimplementation.StartWait(ctx, workspaceClient, true, log)
	if err != nil {
		return err
	}

	log.Infof("building devcontainer")
	defer func() {
		log.Debugf("done building devcontainer")
		log.Infof("cleaning up temporary workspace")
	}()
	_, err = clientimplementation.BuildAgentClient(ctx, clientimplementation.BuildAgentClientOptions{
		WorkspaceClient: workspaceClient,
		CLIOptions:      cmd.CLIOptions,
		AgentCommand:    "build",
		Log:             log,
	})
	return err
}
