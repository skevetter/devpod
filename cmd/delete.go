package cmd

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/cmd/completion"
	"github.com/skevetter/devpod/cmd/flags"
	client2 "github.com/skevetter/devpod/pkg/client"
	"github.com/skevetter/devpod/pkg/client/clientimplementation"
	"github.com/skevetter/devpod/pkg/config"
	"github.com/skevetter/devpod/pkg/workspace"
	"github.com/skevetter/log"
	"github.com/spf13/cobra"
)

// DeleteCmd holds the delete cmd flags.
type DeleteCmd struct {
	*flags.GlobalFlags
	client2.DeleteOptions
}

// NewDeleteCmd creates a new command.
func NewDeleteCmd(flags *flags.GlobalFlags) *cobra.Command {
	cmd := &DeleteCmd{
		GlobalFlags: flags,
	}
	deleteCmd := &cobra.Command{
		Use:   "delete [flags] [workspace-path|workspace-name]",
		Short: "Deletes an existing workspace",
		Long: `Deletes an existing workspace. You can specify the workspace by its path or name.
If the workspace is not found, you can use the --ignore-not-found flag to treat it as a successful delete.`,
		RunE: func(_ *cobra.Command, args []string) error {
			_, err := clientimplementation.DecodeOptionsFromEnv(clientimplementation.DevPodFlagsDelete, &cmd.DeleteOptions)
			if err != nil {
				return fmt.Errorf("decode up options: %w", err)
			}

			ctx := context.Background()
			devPodConfig, err := config.LoadConfig(cmd.Context, cmd.Provider)
			if err != nil {
				return err
			}

			err = clientimplementation.DecodePlatformOptionsFromEnv(&cmd.Platform)
			if err != nil {
				return fmt.Errorf("decode platform options: %w", err)
			}

			return cmd.Run(ctx, devPodConfig, args)
		},
		ValidArgsFunction: func(rootCmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return completion.GetWorkspaceSuggestions(rootCmd, cmd.Context, cmd.Provider, args, toComplete, cmd.Owner, log.Default)
		},
	}

	deleteCmd.Flags().BoolVar(&cmd.IgnoreNotFound, "ignore-not-found", false, "Treat \"workspace not found\" as a successful delete")
	deleteCmd.Flags().StringVar(&cmd.GracePeriod, "grace-period", "", "The amount of time to give the command to delete the workspace")
	deleteCmd.Flags().BoolVar(&cmd.Force, "force", false, "Delete workspace even if it is not found remotely anymore")
	return deleteCmd
}

// Run runs the command logic.
func (cmd *DeleteCmd) Run(ctx context.Context, devPodConfig *config.Config, args []string) error {
	if len(args) == 0 {
		workspaceName, err := workspace.Delete(ctx, devPodConfig, args, cmd.IgnoreNotFound, cmd.Force, cmd.DeleteOptions, cmd.Owner, log.Default)
		if err != nil {
			return err
		}
		log.WithFields(logrus.Fields{
			"workspace": workspaceName,
		})
		log.Default.Donef("deleted workspace")
		return nil
	}

	for _, arg := range args {
		workspaceName, err := workspace.Delete(ctx, devPodConfig, []string{arg}, cmd.IgnoreNotFound, cmd.Force, cmd.DeleteOptions, cmd.Owner, log.Default)
		if err != nil {
			log.WithFields(logrus.Fields{
				"workspace": arg,
				"err":       err,
			}).Error("failed to delete workspace")
			continue
		}
		log.WithFields(logrus.Fields{
			"workspace": workspaceName,
		})
		log.Default.Donef("deleted workspace")
	}
	return nil
}
