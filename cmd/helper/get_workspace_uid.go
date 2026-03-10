package helper

import (
	"context"
	"fmt"

	"github.com/skevetter/devpod/cmd/flags"
	"github.com/skevetter/devpod/pkg/encoding"
	"github.com/spf13/cobra"
)

type GetWorkspaceUIDCommand struct {
	*flags.GlobalFlags
}

// NewGetWorkspaceUIDCmd creates a new command.
func NewGetWorkspaceUIDCmd(flags *flags.GlobalFlags) *cobra.Command {
	cmd := &GetWorkspaceUIDCommand{
		GlobalFlags: flags,
	}
	shellCmd := &cobra.Command{
		Use:   "get-workspace-uid",
		Short: "Retrieves a workspace uid",
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			return cmd.Run(cobraCmd.Context(), args)
		},
	}

	return shellCmd
}

func (cmd *GetWorkspaceUIDCommand) Run(ctx context.Context, args []string) error {
	fmt.Print(encoding.CreateNewUID("", ""))

	return nil
}
