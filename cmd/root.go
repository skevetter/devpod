package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/cmd/agent"
	"github.com/skevetter/devpod/cmd/completion"
	"github.com/skevetter/devpod/cmd/context"
	"github.com/skevetter/devpod/cmd/flags"
	"github.com/skevetter/devpod/cmd/helper"
	"github.com/skevetter/devpod/cmd/ide"
	"github.com/skevetter/devpod/cmd/machine"
	"github.com/skevetter/devpod/cmd/pro"
	"github.com/skevetter/devpod/cmd/provider"
	"github.com/skevetter/devpod/cmd/use"
	"github.com/skevetter/devpod/cmd/workspace"
	"github.com/skevetter/devpod/pkg/client/clientimplementation"
	"github.com/skevetter/devpod/pkg/config"
	"github.com/skevetter/devpod/pkg/telemetry"
	log2 "github.com/skevetter/log"
	"github.com/skevetter/log/terminal"
	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
	"golang.org/x/crypto/ssh"
)

var globalFlags *flags.GlobalFlags

// NewRootCmd returns a new root command
func NewRootCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "devpod",
		Short:         "DevPod",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cobraCmd *cobra.Command, args []string) error {
			if globalFlags.LogOutput == "json" {
				log2.Default.SetFormat(log2.JSONFormat)
			} else if globalFlags.LogOutput == "raw" {
				log2.Default.SetFormat(log2.RawFormat)
			} else if globalFlags.LogOutput != "plain" {
				return fmt.Errorf("unrecognized log format %s, needs to be either plain, raw or json", globalFlags.LogOutput)
			}

			if globalFlags.Silent {
				log2.Default.SetLevel(logrus.FatalLevel)
			} else if globalFlags.Debug {
				log2.Default.SetLevel(logrus.DebugLevel)
			} else if os.Getenv(clientimplementation.DevPodDebug) == "true" {
				log2.Default.SetLevel(logrus.DebugLevel)
			}

			if globalFlags.DevPodHome != "" {
				_ = os.Setenv(config.DEVPOD_HOME, globalFlags.DevPodHome)
			}

			devPodConfig, err := config.LoadConfig(globalFlags.Context, globalFlags.Provider)
			if err == nil {
				telemetry.StartCLI(devPodConfig, cobraCmd)
			}

			return nil
		},
		PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
			if globalFlags.DevPodHome != "" {
				_ = os.Unsetenv(config.DEVPOD_HOME)
			}

			return nil
		},
	}
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	// build the root command
	rootCmd := BuildRoot()

	// execute command
	err := rootCmd.Execute()
	telemetry.CollectorCLI.RecordCLI(err)
	telemetry.CollectorCLI.Flush()
	if err != nil {
		//nolint:all
		if sshExitErr, ok := err.(*ssh.ExitError); ok {
			os.Exit(sshExitErr.ExitStatus())
		}

		//nolint:all
		if execExitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(execExitErr.ExitCode())
		}

		if globalFlags.Debug {
			log2.Default.Fatalf("%+v", err)
		} else {
			if rootCmd.Annotations == nil || rootCmd.Annotations[agent.AgentExecutedAnnotation] != "true" {
				if terminal.IsTerminalIn {
					log2.Default.Error("Try using the --debug flag to see a more verbose output")
				} else if os.Getenv(telemetry.UIEnvVar) == "true" {
					log2.Default.Error("Try enabling Debug mode under Settings to see a more verbose output")
				}
			}
			log2.Default.Fatal(err)
		}
	}
}

// BuildRoot creates a new root command from the
func BuildRoot() *cobra.Command {
	rootCmd := NewRootCmd()
	persistentFlags := rootCmd.PersistentFlags()
	globalFlags = flags.SetGlobalFlags(persistentFlags)
	_ = completion.RegisterFlagCompletionFuns(rootCmd, globalFlags)

	rootCmd.AddCommand(agent.NewAgentCmd(globalFlags))
	rootCmd.AddCommand(provider.NewProviderCmd(globalFlags))
	rootCmd.AddCommand(use.NewUseCmd(globalFlags))
	rootCmd.AddCommand(helper.NewHelperCmd(globalFlags))
	rootCmd.AddCommand(ide.NewIDECmd(globalFlags))
	rootCmd.AddCommand(machine.NewMachineCmd(globalFlags))
	rootCmd.AddCommand(context.NewContextCmd(globalFlags))
	rootCmd.AddCommand(pro.NewProCmd(globalFlags, log2.Default))
	rootCmd.AddCommand(workspace.NewWorkspaceCmd(globalFlags))
	rootCmd.AddCommand(NewUpCmd(globalFlags))
	rootCmd.AddCommand(NewDeleteCmd(globalFlags))
	rootCmd.AddCommand(NewSSHCmd(globalFlags))
	rootCmd.AddCommand(NewVersionCmd())
	rootCmd.AddCommand(NewStopCmd(globalFlags))
	rootCmd.AddCommand(NewListCmd(globalFlags))
	rootCmd.AddCommand(NewStatusCmd(globalFlags))
	rootCmd.AddCommand(NewBuildCmd(globalFlags))
	rootCmd.AddCommand(NewLogsDaemonCmd(globalFlags))
	rootCmd.AddCommand(NewExportCmd(globalFlags))
	rootCmd.AddCommand(NewImportCmd(globalFlags))
	rootCmd.AddCommand(NewLogsCmd(globalFlags))
	rootCmd.AddCommand(NewUpgradeCmd())
	rootCmd.AddCommand(NewTroubleshootCmd(globalFlags))
	rootCmd.AddCommand(NewPingCmd(globalFlags))

	inheritCommandFlagsFromEnvironment(rootCmd)

	return rootCmd
}

func inheritCommandFlagsFromEnvironment(cmd *cobra.Command) {
	inheritFlagsFromEnvironment(cmd.Flags())
	inheritFlagsFromEnvironment(cmd.PersistentFlags())

	for _, sub := range cmd.Commands() {
		inheritCommandFlagsFromEnvironment(sub)
	}
}

// Inherits default values for all flags that have a corresponding environment variable set.
func inheritFlagsFromEnvironment(flags *flag.FlagSet) {
	flags.VisitAll(func(flag *flag.Flag) {
		// calculate environment variable name from flag name
		suffix := strings.ToUpper(strings.ReplaceAll(flag.Name, "-", "_"))

		// do not prepend "DEVPOD_" to the environment variable name if the flag name starts with "devpod"
		// (applies to one flag - "devpod-home").
		var environmentVariable string
		if strings.HasPrefix(suffix, "DEVPOD_") {
			environmentVariable = suffix
		} else {
			environmentVariable = "DEVPOD_" + suffix
		}

		if value, exists := os.LookupEnv(environmentVariable); exists {
			// set the variable holding the flag's value to the default supplied by the environment
			err := flag.Value.Set(value)
			if err != nil {
				log2.Default.Fatalf("failed to set flag %s from the environment variable %s with value %s: %+v", flag.Name, environmentVariable, value, err)
			}
			// reflect this default in the usage output
			flag.DefValue = value
		}

		// add note about environment variable to usage, but only if it is not there yet -
		// in case we visit the same flag more than once.
		usageAddition := ". You can also use " + environmentVariable + " to set this"
		if !strings.HasSuffix(flag.Usage, usageAddition) {
			flag.Usage = flag.Usage + usageAddition
		}
	})
}
