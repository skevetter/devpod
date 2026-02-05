package container

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func NewHealthCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "Check if the agent daemon is healthy",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := os.Stat("/tmp/devpod-daemon.pid"); err != nil {
				return fmt.Errorf("daemon not running")
			}
			return nil
		},
	}
}
