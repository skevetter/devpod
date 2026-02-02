package helper

import (
	"bytes"
	"context"
	"fmt"

	"github.com/skevetter/devpod/cmd/flags"
	"github.com/skevetter/devpod/pkg/provider"
	"github.com/skevetter/devpod/pkg/workspace"
	"github.com/skevetter/log"
	"github.com/spf13/cobra"
)

type GetProviderNameCmd struct {
	*flags.GlobalFlags
}

// NewGetProviderNameCmd creates a new command
func NewGetProviderNameCmd(flags *flags.GlobalFlags) *cobra.Command {
	cmd := &GetProviderNameCmd{
		GlobalFlags: flags,
	}
	shellCmd := &cobra.Command{
		Use:   "get-provider-name",
		Short: "Retrieves a provider name",
		RunE: func(_ *cobra.Command, args []string) error {
			return cmd.Run(context.Background(), args)
		},
	}

	return shellCmd
}

func (cmd *GetProviderNameCmd) Run(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("provider is missing")
	}

	providerRaw, _, err := workspace.ResolveProvider(args[0], log.Default.ErrorStreamOnly())
	if err != nil {
		return fmt.Errorf("resolve provider: %w", err)
	}

	providerConfig, err := provider.ParseProvider(bytes.NewReader(providerRaw))
	if err != nil {
		return fmt.Errorf("parse provider: %w", err)
	}

	fmt.Print(providerConfig.Name)
	return nil
}
