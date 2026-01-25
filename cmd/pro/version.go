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

// VersionCmd holds the cmd flags
type VersionCmd struct {
	*flags.GlobalFlags
	Log log.Logger

	Host string
}

// NewVersionCmd creates a new command
func NewVersionCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &VersionCmd{
		GlobalFlags: globalFlags,
		Log:         log.GetInstance(),
	}
	c := &cobra.Command{
		Use:    "version",
		Short:  "Get version",
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

func (cmd *VersionCmd) Run(ctx context.Context, devPodConfig *config.Config, providerConfig *provider.ProviderConfig) error {
	opts := devPodConfig.ProviderOptions(providerConfig.Name)
	opts[provider.PROVIDER_ID] = config.OptionValue{Value: providerConfig.Name}
	opts[provider.PROVIDER_CONTEXT] = config.OptionValue{Value: cmd.Context}

	var buf bytes.Buffer
	// ignore --debug because we tunnel json through stdio
	cmd.Log.SetLevel(logrus.InfoLevel)

	err := clientimplementation.RunCommandWithBinaries(clientimplementation.CommandOptions{
		Ctx:     ctx,
		Name:    "getVersion",
		Command: providerConfig.Exec.Proxy.Get.Version,
		Context: devPodConfig.DefaultContext,
		Options: opts,
		Config:  providerConfig,
		Stdout:  &buf,
		Log:     cmd.Log,
	})
	if err != nil {
		return fmt.Errorf("get version %w", err)
	}

	fmt.Print(buf.String())

	return nil
}
