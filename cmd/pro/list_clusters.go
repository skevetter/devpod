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

// ListClustersCmd holds the cmd flags
type ListClustersCmd struct {
	*flags.GlobalFlags
	Log log.Logger

	Host    string
	Project string
}

// NewListClustersCmd creates a new command
func NewListClustersCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &ListClustersCmd{
		GlobalFlags: globalFlags,
		Log:         log.GetInstance(),
	}
	c := &cobra.Command{
		Use:    "list-clusters",
		Short:  "List clusters",
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
	c.Flags().StringVar(&cmd.Project, "project", "", "The project to use")
	_ = c.MarkFlagRequired("project")

	return c
}

func (cmd *ListClustersCmd) Run(ctx context.Context, devPodConfig *config.Config, provider *provider.ProviderConfig) error {
	opts := devPodConfig.ProviderOptions(provider.Name)
	opts[platform.ProjectEnv] = config.OptionValue{Value: cmd.Project}

	// ignore --debug because we tunnel json through stdio
	cmd.Log.SetLevel(logrus.InfoLevel)

	var buf bytes.Buffer
	err := clientimplementation.RunCommandWithBinaries(clientimplementation.CommandOptions{
		Ctx:     ctx,
		Name:    "listClusters",
		Command: provider.Exec.Proxy.List.Clusters,
		Context: devPodConfig.DefaultContext,
		Options: opts,
		Config:  provider,
		Stdout:  &buf,
		Log:     cmd.Log,
	})
	if err != nil {
		return fmt.Errorf("list clusters with provider \"%s\": %w", provider.Name, err)
	}

	fmt.Println(buf.String())

	return nil
}
