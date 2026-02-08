package workspace

import (
	"github.com/skevetter/devpod/cmd/flags"
	"github.com/spf13/cobra"
)

func NewWorkspaceCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	workspaceCmd := &cobra.Command{
		Use:   "workspace",
		Short: "Manage workspaces",
	}

	workspaceCmd.AddCommand(NewRebindCmd(globalFlags))
	return workspaceCmd
}
