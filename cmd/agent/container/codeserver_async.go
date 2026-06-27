//nolint:dupl // CodeServerAsyncCmd is intentionally parallel to VSCodeWebAsyncCmd but uses a distinct IDE package
package container

import (
	"encoding/json"

	"github.com/skevetter/devpod/cmd/flags"
	"github.com/skevetter/devpod/pkg/compress"
	"github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/skevetter/devpod/pkg/ide/codeserver"
	"github.com/skevetter/log"
	"github.com/spf13/cobra"
)

// CodeServerAsyncCmd holds the cmd flags.
type CodeServerAsyncCmd struct {
	*flags.GlobalFlags

	SetupInfo string
}

// NewCodeServerAsyncCmd creates a new command.
func NewCodeServerAsyncCmd() *cobra.Command {
	cmd := &CodeServerAsyncCmd{}
	asyncCmd := &cobra.Command{
		Use:   "codeserver-async",
		Short: "Installs code-server extensions",
		Args:  cobra.NoArgs,
		RunE:  cmd.Run,
	}
	asyncCmd.Flags().StringVar(&cmd.SetupInfo, "setup-info", "", "The container setup info")
	_ = asyncCmd.MarkFlagRequired("setup-info")
	return asyncCmd
}

// Run runs the command logic.
func (cmd *CodeServerAsyncCmd) Run(_ *cobra.Command, _ []string) error {
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
	return codeserver.NewCodeServerServer(vsCodeConfiguration.Extensions, "", user, "", "", nil, log.Default).
		InstallExtensions()
}
