package gpg

import (
	"os"
	"os/exec"

	client2 "github.com/skevetter/devpod/pkg/client"
	devssh "github.com/skevetter/devpod/pkg/ssh"
	"github.com/skevetter/log"
)

// ForwardAgent starts a background SSH connection that forwards the local GPG agent.
func ForwardAgent(client client2.BaseWorkspaceClient, logger log.Logger) error {
	logger.Debug("gpg forwarding enabled, performing immediately")

	execPath, err := os.Executable()
	if err != nil {
		return err
	}

	remoteUser, err := devssh.GetUser(
		client.WorkspaceConfig().ID,
		client.WorkspaceConfig().SSHConfigPath,
		client.WorkspaceConfig().SSHConfigIncludePath,
	)
	if err != nil {
		remoteUser = "root"
	}

	logger.Info("forwarding gpg-agent")

	args := buildForwardArgs(remoteUser, client.Context(), client.Workspace())

	go func() {
		//nolint:gosec // execPath comes from os.Executable()
		if runErr := exec.Command(execPath, args...).Run(); runErr != nil {
			logger.Errorf("failure in forwarding gpg-agent: %v", runErr)
		}
	}()

	return nil
}

func buildForwardArgs(user, context, workspace string) []string {
	return []string{
		"ssh",
		"--gpg-agent-forwarding=true",
		"--agent-forwarding=true",
		"--start-services=true",
		"--user",
		user,
		"--context",
		context,
		workspace,
		"--log-output=raw",
		"--command", "sleep infinity",
	}
}
