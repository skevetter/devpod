package provider

import (
	"os"

	"github.com/skevetter/devpod/cmd/agent"
	"github.com/skevetter/devpod/cmd/pro/flags"
	"github.com/skevetter/devpod/cmd/pro/provider/create"
	"github.com/skevetter/devpod/cmd/pro/provider/get"
	"github.com/skevetter/devpod/cmd/pro/provider/list"
	"github.com/skevetter/devpod/cmd/pro/provider/update"
	"github.com/skevetter/devpod/cmd/pro/provider/watch"
	"github.com/skevetter/devpod/pkg/client/clientimplementation"
	"github.com/skevetter/devpod/pkg/platform"
	"github.com/skevetter/devpod/pkg/platform/client"
	"github.com/skevetter/devpod/pkg/telemetry"
	"github.com/skevetter/log"

	"github.com/spf13/cobra"
)

// NewProProviderCmd creates a new cobra command
func NewProProviderCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	c := &cobra.Command{
		Use:    "provider",
		Short:  "DevPod Pro provider commands",
		Args:   cobra.NoArgs,
		Hidden: true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if (globalFlags.Config == "" || globalFlags.Config == client.DefaultCacheConfig) && os.Getenv("LOFT_CONFIG") != "" {
				globalFlags.Config = os.Getenv(platform.ConfigEnv)
			}

			log.Default.SetFormat(log.JSONFormat)

			if os.Getenv(clientimplementation.DevPodDebug) == "true" {
				globalFlags.Debug = true
			}

			// Disable debug hints if we execute pro commands from DevPod Desktop
			// We're reusing the agent.AgentExecutedAnnotation for simplicity, could rename in the future
			if os.Getenv(telemetry.UIEnvVar) == "true" {
				cmd.VisitParents(func(c *cobra.Command) {
					// find the root command
					if c.Name() == "devpod" {
						if c.Annotations == nil {
							c.Annotations = map[string]string{}
						}
						c.Annotations[agent.AgentExecutedAnnotation] = "true"
					}
				})
			}
		},
	}

	c.AddCommand(list.NewCmd(globalFlags))
	c.AddCommand(watch.NewCmd(globalFlags))
	c.AddCommand(create.NewCmd(globalFlags))
	c.AddCommand(get.NewCmd(globalFlags))
	c.AddCommand(update.NewCmd(globalFlags))
	c.AddCommand(NewHealthCmd(globalFlags))

	c.AddCommand(NewUpCmd(globalFlags))
	c.AddCommand(NewStopCmd(globalFlags))
	c.AddCommand(NewSshCmd(globalFlags))
	c.AddCommand(NewStatusCmd(globalFlags))
	c.AddCommand(NewDeleteCmd(globalFlags))
	c.AddCommand(NewRebuildCmd(globalFlags))
	return c
}
