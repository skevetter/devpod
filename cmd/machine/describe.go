package machine

import (
	"context"
	"fmt"
	"os"

	"github.com/skevetter/devpod/cmd/flags"
	"github.com/skevetter/devpod/pkg/config"
	"github.com/skevetter/devpod/pkg/workspace"
	"github.com/skevetter/log"
	"github.com/spf13/cobra"
)

// DescribeCmd holds the configuration.
type DescribeCmd struct {
	*flags.GlobalFlags
}

// NewDescribeCmd creates a new describe command.
func NewDescribeCmd(flags *flags.GlobalFlags) *cobra.Command {
	cmd := &DescribeCmd{
		GlobalFlags: flags,
	}
	describeCmd := &cobra.Command{
		Use:   "describe [name]",
		Short: "Retrieves the description of an existing machine",
		RunE: func(_ *cobra.Command, args []string) error {
			return cmd.Run(context.Background(), args)
		},
	}

	return describeCmd
}

// Run runs the command logic.
func (cmd *DescribeCmd) Run(ctx context.Context, args []string) error {
	devPodConfig, err := config.LoadConfig(cmd.Context, cmd.Provider)
	if err != nil {
		return err
	}

	machineClient, err := workspace.GetMachine(devPodConfig, args, log.Default)
	if err != nil {
		return err
	}

	// get description
	machineDescription, err := machineClient.Describe(ctx)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintln(os.Stdout, machineDescription)

	return nil
}
