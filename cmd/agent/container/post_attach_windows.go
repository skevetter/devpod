//go:build windows

package container

import (
	"github.com/skevetter/devpod/cmd/flags"
	"github.com/spf13/cobra"
)

func NewPostAttachCmd(flags *flags.GlobalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "post-attach",
		Short: "Runs postAttachCommand lifecycle hooks",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			panic("Windows Containers are not supported")
		},
	}
}
