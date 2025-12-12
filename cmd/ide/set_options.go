package ide

import (
	"context"
	"fmt"
	"strings"

	"github.com/skevetter/devpod/cmd/flags"
	"github.com/skevetter/devpod/pkg/config"
	"github.com/skevetter/devpod/pkg/ide/ideparse"
	"github.com/spf13/cobra"
)

// SetOptionsCmd holds the setOptions cmd flags
type SetOptionsCmd struct {
	flags.GlobalFlags

	Options []string
}

// NewSetOptionsCmd creates a new command
func NewSetOptionsCmd(flags *flags.GlobalFlags) *cobra.Command {
	cmd := &SetOptionsCmd{
		GlobalFlags: *flags,
	}
	setOptionsCmd := &cobra.Command{
		Use:   "set-options",
		Short: "Configure ide options",
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("please specify the ide")
			}

			return cmd.Run(context.Background(), args[0])
		},
	}

	setOptionsCmd.Flags().StringArrayVarP(&cmd.Options, "option", "o", []string{}, "IDE option in the form KEY=VALUE")
	return setOptionsCmd
}

// Run runs the command logic
func (cmd *SetOptionsCmd) Run(ctx context.Context, ide string) error {
	devPodConfig, err := config.LoadConfig(cmd.Context, cmd.Provider)
	if err != nil {
		return err
	}

	ide = strings.ToLower(ide)
	ideOptions, err := ideparse.GetIDEOptions(ide)
	if err != nil {
		return err
	}

	// check if there are setOptionsr options set
	if len(cmd.Options) > 0 {
		err = setOptions(devPodConfig, ide, cmd.Options, ideOptions)
		if err != nil {
			return err
		}
	}

	err = config.SaveConfig(devPodConfig)
	if err != nil {
		return fmt.Errorf("save config %w", err)
	}

	return nil
}
