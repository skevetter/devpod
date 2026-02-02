package ide

import (
	"context"
	"fmt"
	"maps"
	"strings"

	"github.com/skevetter/devpod/cmd/flags"
	"github.com/skevetter/devpod/pkg/config"
	"github.com/skevetter/devpod/pkg/ide"
	"github.com/skevetter/devpod/pkg/ide/ideparse"
	options2 "github.com/skevetter/devpod/pkg/options"
	"github.com/spf13/cobra"
)

// UseCmd holds the use cmd flags
type UseCmd struct {
	*flags.GlobalFlags

	Options []string
}

// NewUseCmd creates a new command
func NewUseCmd(flags *flags.GlobalFlags) *cobra.Command {
	cmd := &UseCmd{
		GlobalFlags: flags,
	}
	useCmd := &cobra.Command{
		Use:   "use",
		Short: "Configure the default IDE to use (list available IDEs with 'devpod ide list')",
		Long: `Configure the default IDE to use

Available IDEs can be listed with 'devpod ide list'`,
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("please specify the ide to use, list available IDEs with 'devpod ide list'")
			}

			return cmd.Run(context.Background(), args[0])
		},
	}

	useCmd.Flags().StringArrayVarP(&cmd.Options, "option", "o", []string{}, "IDE option in the form KEY=VALUE")
	return useCmd
}

// Run runs the command logic
func (cmd *UseCmd) Run(ctx context.Context, ide string) error {
	devPodConfig, err := config.LoadConfig(cmd.Context, cmd.Provider)
	if err != nil {
		return err
	}

	ide = strings.ToLower(ide)
	ideOptions, err := ideparse.GetIDEOptions(ide)
	if err != nil {
		return err
	}

	// check if there are user options set
	if len(cmd.Options) > 0 {
		err = setOptions(devPodConfig, ide, cmd.Options, ideOptions)
		if err != nil {
			return err
		}
	}

	devPodConfig.Current().DefaultIDE = ide
	err = config.SaveConfig(devPodConfig)
	if err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	return nil
}

func setOptions(devPodConfig *config.Config, ide string, userOptions []string, ideOptions ide.Options) error {
	userOptions = options2.InheritOptionsFromEnvironment(
		userOptions,
		ideOptions,
		"DEVPOD_IDE_"+ide+"_",
	)

	optionValues, err := ideparse.ParseOptions(userOptions, ideOptions)
	if err != nil {
		return err
	}

	if devPodConfig.Current().IDEs == nil {
		devPodConfig.Current().IDEs = map[string]*config.IDEConfig{}
	}

	newValues := map[string]config.OptionValue{}
	if devPodConfig.Current().IDEs[ide] != nil {
		maps.Copy(newValues, devPodConfig.Current().IDEs[ide].Options)
	}
	maps.Copy(newValues, optionValues)

	devPodConfig.Current().IDEs[ide] = &config.IDEConfig{
		Options: newValues,
	}
	return nil
}
