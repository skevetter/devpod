package provider

import (
	"context"
	"fmt"
	"io"

	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/cmd/completion"
	"github.com/skevetter/devpod/cmd/flags"
	"github.com/skevetter/devpod/pkg/client/clientimplementation"
	"github.com/skevetter/devpod/pkg/config"
	options2 "github.com/skevetter/devpod/pkg/options"
	provider2 "github.com/skevetter/devpod/pkg/provider"
	"github.com/skevetter/devpod/pkg/workspace"
	"github.com/skevetter/log"
	"github.com/spf13/cobra"
)

// UseCmd holds the use cmd flags
type UseCmd struct {
	*flags.GlobalFlags

	Reconfigure   bool
	SingleMachine bool
	Options       []string

	// only for testing
	SkipInit bool
}

// NewUseCmd creates a new command
func NewUseCmd(flags *flags.GlobalFlags) *cobra.Command {
	cmd := &UseCmd{
		GlobalFlags: flags,
	}
	useCmd := &cobra.Command{
		Use:   "use [name]",
		Short: "Configure an existing provider and set as default",
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("please specify the provider to use")
			}

			return cmd.Run(context.Background(), args[0])
		},
		ValidArgsFunction: func(rootCmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return completion.GetProviderSuggestions(rootCmd, cmd.Context, cmd.Provider, args, toComplete, cmd.Owner, log.Default)
		},
	}

	AddFlags(useCmd, cmd)
	return useCmd
}

func AddFlags(useCmd *cobra.Command, cmd *UseCmd) {
	useCmd.Flags().BoolVar(&cmd.SingleMachine, "single-machine", false, "If enabled will use a single machine for all workspaces")
	useCmd.Flags().BoolVar(&cmd.Reconfigure, "reconfigure", false, "If enabled will not merge existing provider config")
	useCmd.Flags().StringArrayVarP(&cmd.Options, "option", "o", []string{}, "Provider option in the form KEY=VALUE")

	useCmd.Flags().BoolVar(&cmd.SkipInit, "skip-init", false, "ONLY FOR TESTING: If true will skip init")
	_ = useCmd.Flags().MarkHidden("skip-init")
}

// Run runs the command logic
func (cmd *UseCmd) Run(ctx context.Context, providerName string) error {
	devPodConfig, err := config.LoadConfig(cmd.Context, cmd.Provider)
	if err != nil {
		return err
	}

	providerWithOptions, err := workspace.FindProvider(devPodConfig, providerName, log.Default)
	if err != nil {
		return err
	}

	// should reconfigure?
	shouldReconfigure := cmd.Reconfigure || len(cmd.Options) > 0 || providerWithOptions.State == nil || cmd.SingleMachine
	if shouldReconfigure {
		return ConfigureProvider(ctx, ProviderOptionsConfig{
			Provider:       providerWithOptions.Config,
			Context:        devPodConfig.DefaultContext,
			UserOptions:    cmd.Options,
			Reconfigure:    cmd.Reconfigure,
			SkipRequired:   false,
			SkipInit:       cmd.SkipInit,
			SkipSubOptions: false,
			SingleMachine:  &cmd.SingleMachine,
			Log:            log.Default,
		})
	} else {
		log.Default.Infof("To reconfigure provider %s, run with '--reconfigure' to reconfigure the provider", providerWithOptions.Config.Name)
	}

	// set options
	defaultContext := devPodConfig.Current()
	defaultContext.DefaultProvider = providerWithOptions.Config.Name

	// save provider config
	err = config.SaveConfig(devPodConfig)
	if err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	// print success message
	log.WithFields(logrus.Fields{
		"providerName": providerWithOptions.Config.Name,
	}).Done("switched default provider")
	return nil
}

type ProviderOptionsConfig struct {
	Provider       *provider2.ProviderConfig
	Context        string
	UserOptions    []string
	Reconfigure    bool
	SkipRequired   bool
	SkipInit       bool
	SkipSubOptions bool
	SingleMachine  *bool
	Log            log.Logger
}

func ConfigureProvider(ctx context.Context, cfg ProviderOptionsConfig) error {
	devPodConfig, err := configureProviderOptions(ctx, cfg)
	if err != nil {
		return err
	}

	// set options
	defaultContext := devPodConfig.Current()
	defaultContext.DefaultProvider = cfg.Provider.Name

	// save provider config
	err = config.SaveConfig(devPodConfig)
	if err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	cfg.Log.Donef("configured provider %s", cfg.Provider.Name)
	return nil
}

func mergeExistingOptions(options map[string]string, existingOptions map[string]config.OptionValue) {
	for k, v := range existingOptions {
		if _, ok := options[k]; !ok && v.UserProvided {
			options[k] = v.Value
		}
	}
}

func configureProviderOptions(ctx context.Context, cfg ProviderOptionsConfig) (*config.Config, error) {
	devPodConfig, err := config.LoadConfig(cfg.Context, "")
	if err != nil {
		return nil, err
	}

	cfg.UserOptions = options2.InheritOptionsFromEnvironment(
		cfg.UserOptions,
		cfg.Provider.Options,
		"DEVPOD_PROVIDER_"+cfg.Provider.Name+"_",
	)

	// parse options
	options, err := provider2.ParseOptions(cfg.UserOptions)
	if err != nil {
		return nil, fmt.Errorf("parse options: %w", err)
	}

	// merge with old values
	if !cfg.Reconfigure {
		mergeExistingOptions(options, devPodConfig.ProviderOptions(cfg.Provider.Name))
	}

	// fill defaults
	devPodConfig, err = options2.ResolveOptions(
		ctx, devPodConfig, cfg.Provider, options,
		cfg.SkipRequired, cfg.SkipSubOptions, cfg.SingleMachine, cfg.Log,
	)
	if err != nil {
		return nil, fmt.Errorf("resolve options: %w", err)
	}

	// run init command
	if !cfg.SkipInit {
		stdout := cfg.Log.Writer(logrus.InfoLevel, false)
		defer func() { _ = stdout.Close() }()

		stderr := cfg.Log.Writer(logrus.ErrorLevel, false)
		defer func() { _ = stderr.Close() }()

		err = initProvider(ctx, devPodConfig, cfg.Provider, stdout, stderr)
		if err != nil {
			return nil, err
		}
	}

	return devPodConfig, nil
}

func initProvider(ctx context.Context, devPodConfig *config.Config, provider *provider2.ProviderConfig, stdout, stderr io.Writer) error {
	err := clientimplementation.RunCommandWithBinaries(clientimplementation.CommandOptions{
		Ctx:     ctx,
		Name:    "init",
		Command: provider.Exec.Init,
		Context: devPodConfig.DefaultContext,
		Options: devPodConfig.ProviderOptions(provider.Name),
		Config:  provider,
		Stdout:  stdout,
		Stderr:  stderr,
		Log:     log.Default,
	})
	if err != nil {
		return fmt.Errorf("init: %w", err)
	}
	if devPodConfig.Current().Providers == nil {
		devPodConfig.Current().Providers = map[string]*config.ProviderConfig{}
	}
	if devPodConfig.Current().Providers[provider.Name] == nil {
		devPodConfig.Current().Providers[provider.Name] = &config.ProviderConfig{}
	}
	devPodConfig.Current().Providers[provider.Name].Initialized = true
	return nil
}
