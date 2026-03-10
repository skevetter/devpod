package machine

import (
	"context"

	"github.com/skevetter/devpod/cmd/flags"
	"github.com/skevetter/devpod/pkg/config"
	"github.com/skevetter/devpod/pkg/workspace"
	"github.com/skevetter/log"
	"github.com/spf13/cobra"
)

// StartCmd holds the configuration.
type StartCmd struct {
	*flags.GlobalFlags
}

// NewStartCmd creates a new start command.
func NewStartCmd(flags *flags.GlobalFlags) *cobra.Command {
	cmd := &StartCmd{
		GlobalFlags: flags,
	}
	startCmd := &cobra.Command{
		Use:   "start [name]",
		Short: "Starts an existing machine",
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			return cmd.Run(cobraCmd.Context(), args)
		},
	}

	return startCmd
}

// Run runs the command logic.
func (cmd *StartCmd) Run(ctx context.Context, args []string) error {
	devPodConfig, err := config.LoadConfig(cmd.Context, cmd.Provider)
	if err != nil {
		return err
	}

	machineClient, err := workspace.GetMachine(devPodConfig, args, log.Default)
	if err != nil {
		return err
	}

	err = machineClient.Start(ctx)
	if err != nil {
		return err
	}

	return nil
}
