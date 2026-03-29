package loftconfig

import (
	"fmt"
	"os/exec"

	pkgconfig "github.com/skevetter/devpod/pkg/config"
	"github.com/skevetter/devpod/pkg/platform/client"
	"github.com/skevetter/log"
)

func AuthDevpodCliToPlatform(config *client.Config, logger log.Logger) error {
	cmd := exec.Command( // #nosec G204 -- binary name is a compile-time constant
		pkgconfig.BinaryName,
		"pro",
		"login",
		"--access-key",
		config.AccessKey,
		config.Host,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		logger.Debugf(
			"Failed executing `%s pro login`: %w, output: %s",
			pkgconfig.BinaryName,
			err,
			out,
		)
		return fmt.Errorf(
			"error executing '%s pro login' command: %w, host: %v",
			pkgconfig.BinaryName,
			err,
			config.Host,
		)
	}

	return nil
}

func AuthVClusterCliToPlatform(config *client.Config, logger log.Logger) error {
	// Check if vcluster is available inside the workspace
	if _, err := exec.LookPath("vcluster"); err != nil {
		logger.Debugf("'vcluster' command is not available")
		return nil
	}

	cmd := exec.Command("vcluster", "login", "--access-key", config.AccessKey, config.Host)
	out, err := cmd.CombinedOutput()
	if err != nil {
		logger.Debugf("Failed executing `vcluster login` : %w, output: %s", err, out)
		return fmt.Errorf("error executing 'vcluster login' command: %w", err)
	}

	return nil
}
