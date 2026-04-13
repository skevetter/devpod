//go:build !windows

package container

import (
	"context"
	"encoding/json"

	"github.com/skevetter/devpod/cmd/flags"
	"github.com/skevetter/devpod/pkg/compress"
	"github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/skevetter/devpod/pkg/devcontainer/setup"
	"github.com/skevetter/log"
	"github.com/spf13/cobra"
)

// PostAttachCmd runs postAttachCommand hooks as a detached background process.
type PostAttachCmd struct {
	*flags.GlobalFlags
	SetupInfo string
}

// NewPostAttachCmd creates a new command.
func NewPostAttachCmd(flags *flags.GlobalFlags) *cobra.Command {
	cmd := &PostAttachCmd{
		GlobalFlags: flags,
	}
	postAttachCmd := &cobra.Command{
		Use:   "post-attach",
		Short: "Runs postAttachCommand lifecycle hooks",
		Args:  cobra.NoArgs,
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			return cmd.Run(cobraCmd.Context())
		},
	}
	postAttachCmd.Flags().StringVar(&cmd.SetupInfo, "setup-info", "", "The container setup info")
	_ = postAttachCmd.MarkFlagRequired("setup-info")
	return postAttachCmd
}

// Run runs the postAttachCommand lifecycle hooks.
func (cmd *PostAttachCmd) Run(ctx context.Context) error {
	logger := log.Default

	decompressed, err := compress.Decompress(cmd.SetupInfo)
	if err != nil {
		return err
	}

	setupInfo := &config.Result{}
	if err := json.Unmarshal([]byte(decompressed), setupInfo); err != nil {
		return err
	}

	logger.Debugf("running postAttachCommand hooks")
	if err := setup.RunPostAttachHooks(ctx, setupInfo, logger); err != nil {
		logger.Errorf("postAttachCommand failed: %v", err)
	}

	return nil
}
