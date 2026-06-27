//nolint:dupl // VSCodeWebAsyncCmd is intentionally parallel to CodeServerAsyncCmd but uses a distinct IDE package
package container

import (
	"encoding/json"

	"github.com/skevetter/devpod/cmd/flags"
	"github.com/skevetter/devpod/pkg/compress"
	"github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/skevetter/devpod/pkg/ide/vscodeweb"
	"github.com/skevetter/log"
	"github.com/spf13/cobra"
)

// VSCodeWebAsyncCmd holds the cmd flags.
type VSCodeWebAsyncCmd struct {
	*flags.GlobalFlags

	SetupInfo string
}

// NewVSCodeWebAsyncCmd creates a new command.
func NewVSCodeWebAsyncCmd() *cobra.Command {
	cmd := &VSCodeWebAsyncCmd{}
	asyncCmd := &cobra.Command{
		Use:   "vscodeweb-async",
		Short: "Installs vscode-web extensions",
		Args:  cobra.NoArgs,
		RunE:  cmd.Run,
	}
	asyncCmd.Flags().StringVar(&cmd.SetupInfo, "setup-info", "", "The container setup info")
	_ = asyncCmd.MarkFlagRequired("setup-info")
	return asyncCmd
}

// Run runs the command logic.
func (cmd *VSCodeWebAsyncCmd) Run(_ *cobra.Command, _ []string) error {
	log.Default.Debugf("Start setting up container...")
	decompressed, err := compress.Decompress(cmd.SetupInfo)
	if err != nil {
		return err
	}

	setupInfo := &config.Result{}
	err = json.Unmarshal([]byte(decompressed), setupInfo)
	if err != nil {
		return err
	}

	vsCodeConfiguration := config.GetVSCodeConfiguration(setupInfo.MergedConfig)
	user := config.GetRemoteUser(setupInfo)
	return vscodeweb.NewVSCodeWebServer(vsCodeConfiguration.Extensions, "", user, "", "", nil, log.Default).
		InstallExtensions()
}
