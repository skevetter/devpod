package pro

import (
	"bytes"
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/cmd/pro/flags"
	"github.com/skevetter/devpod/pkg/client/clientimplementation"
	"github.com/skevetter/devpod/pkg/config"
	"github.com/skevetter/devpod/pkg/provider"
	"github.com/skevetter/log"
	"github.com/spf13/cobra"
)

// ListProjectsCmd holds the cmd flags.
type ListProjectsCmd struct {
	*flags.GlobalFlags
	Log log.Logger

	Host string
}

// NewListProjectsCmd creates a new command.
func NewListProjectsCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &ListProjectsCmd{
		GlobalFlags: globalFlags,
		Log:         log.GetInstance(),
	}
	c := &cobra.Command{
		Use:    "list-projects",
		Short:  "List projects",
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

	return c
}

func (cmd *ListProjectsCmd) Run(ctx context.Context, devPodConfig *config.Config, provider *provider.ProviderConfig) error {
	var buf bytes.Buffer
	// ignore --debug because we tunnel json through stdio
	cmd.Log.SetLevel(logrus.InfoLevel)

	err := clientimplementation.RunCommandWithBinaries(clientimplementation.CommandOptions{
		Ctx:     ctx,
		Name:    "listProjects",
		Command: provider.Exec.Proxy.List.Projects,
		Context: devPodConfig.DefaultContext,
		Options: devPodConfig.ProviderOptions(provider.Name),
		Config:  provider,
		Stdout:  &buf,
		Log:     cmd.Log,
	})
	if err != nil {
		return fmt.Errorf("watch workspaces with provider \"%s\": %w", provider.Name, err)
	}

	fmt.Println(buf.String())

	return nil
}
