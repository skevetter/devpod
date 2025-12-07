package container

import (
	workspaced "github.com/skevetter/devpod/pkg/daemon/workspace"
	"github.com/spf13/cobra"
)

// NewDaemonCmd creates the daemon cobra command.
func NewDaemonCmd() *cobra.Command {
	d := workspaced.NewDaemon()
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Starts the DevPod network daemon, SSH server and monitors container activity if timeout is set",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			daemon := workspaced.NewDaemon()
			return daemon.Run(cmd, args)
		},
	}

	cmd.Flags().StringVar(&d.Config.Timeout, "timeout", "", "The timeout to stop the container after")

	return cmd
}
