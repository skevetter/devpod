package pro

import (
	"bytes"
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/cmd/pro/flags"
	"github.com/skevetter/devpod/pkg/client/clientimplementation"
	"github.com/skevetter/devpod/pkg/config"
	"github.com/skevetter/devpod/pkg/platform"
	"github.com/skevetter/devpod/pkg/provider"
	"github.com/skevetter/log"
	"github.com/spf13/cobra"
)

// UpdateWorkspaceCmd holds the cmd flags.
type UpdateWorkspaceCmd struct {
	*flags.GlobalFlags
	Log log.Logger

	Host     string
	Instance string
}

// NewUpdateWorkspaceCmd creates a new command.
func NewUpdateWorkspaceCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &UpdateWorkspaceCmd{
		GlobalFlags: globalFlags,
		Log:         log.GetInstance(),
	}
	c := &cobra.Command{
		Use:    "update-workspace",
		Short:  "Update workspace instance",
		Hidden: true,
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			devPodConfig, provider, err := findProProvider(cobraCmd.Context(), cmd.Context, cmd.Provider, cmd.Host, cmd.Log)
			if err != nil {
				return err
			}

			return cmd.Run(cobraCmd.Context(), devPodConfig, provider)
		},
	}

	c.Flags().StringVar(&cmd.Host, "host", "", "The pro instance to use")
	_ = c.MarkFlagRequired("host")
	c.Flags().StringVar(&cmd.Instance, "instance", "", "The workspace instance to update")
	_ = c.MarkFlagRequired("instance")

	return c
}

func (cmd *UpdateWorkspaceCmd) Run(ctx context.Context, devPodConfig *config.Config, provider *provider.ProviderConfig) error {
	opts := devPodConfig.ProviderOptions(provider.Name)
	opts[platform.WorkspaceInstanceEnv] = config.OptionValue{Value: cmd.Instance}

	var buf bytes.Buffer
	// ignore --debug because we tunnel json through stdio
	cmd.Log.SetLevel(logrus.InfoLevel)

	err := clientimplementation.RunCommandWithBinaries(clientimplementation.CommandOptions{
		Ctx:     ctx,
		Name:    "updateWorkspace",
		Command: provider.Exec.Proxy.Update.Workspace,
		Context: devPodConfig.DefaultContext,
		Options: opts,
		Config:  provider,
		Stdout:  &buf,
		Stderr:  cmd.Log.ErrorStreamOnly().Writer(logrus.ErrorLevel, true),
		Log:     cmd.Log,
	})
	if err != nil {
		return fmt.Errorf("update workspace with provider \"%s\": %w", provider.Name, err)
	}

	fmt.Println(buf.String())

	return nil
}
