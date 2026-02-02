package agent

import (
	"context"
	"os"

	dockercredhelpers "github.com/docker/docker-credential-helpers/credentials"
	"github.com/skevetter/devpod/cmd/flags"
	"github.com/skevetter/devpod/pkg/dockercredentials"
	"github.com/skevetter/log"
	"github.com/spf13/cobra"
)

// DockerCredentialsCmd holds the cmd flags
type DockerCredentialsCmd struct {
	*flags.GlobalFlags

	Port int
}

// NewDockerCredentialsCmd creates a new command
func NewDockerCredentialsCmd(flags *flags.GlobalFlags) *cobra.Command {
	cmd := &DockerCredentialsCmd{
		GlobalFlags: flags,
	}
	dockerCredentialsCmd := &cobra.Command{
		Use:   "docker-credentials",
		Short: "Retrieves docker-credentials from the local machine",
		RunE: func(_ *cobra.Command, args []string) error {
			return cmd.Run(context.Background(), args, log.Default.ErrorStreamOnly())
		},
	}
	dockerCredentialsCmd.Flags().IntVar(&cmd.Port, "port", 0, "If specified, will use the given port")
	_ = dockerCredentialsCmd.MarkFlagRequired("port")
	return dockerCredentialsCmd
}

func (cmd *DockerCredentialsCmd) Run(ctx context.Context, args []string, log log.Logger) error {
	helper := dockercredentials.NewHelper(cmd.Port)

	// Get action from args or stdin
	action := ""
	if len(args) > 0 {
		action = args[0]
	}

	return dockercredhelpers.HandleCommand(helper, action, os.Stdin, os.Stdout)
}
