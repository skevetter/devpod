package clientimplementation

import (
	"context"

	managementv1 "github.com/loft-sh/api/v4/pkg/apis/management/v1"
	"github.com/skevetter/devpod/pkg/client"
	"github.com/skevetter/devpod/pkg/config"
	daemon "github.com/skevetter/devpod/pkg/daemon/platform"
	"github.com/skevetter/devpod/pkg/tunnel"
	"github.com/skevetter/log"
	"golang.org/x/crypto/ssh"
)

type StartServicesDaemonOptions struct {
	DevPodConfig *config.Config
	Client       client.DaemonClient
	SSHClient    *ssh.Client
	User         string
	Log          log.Logger
	ForwardPorts bool
	ExtraPorts   []string
}

type credentialConfig struct {
	docker          bool
	git             bool
	gitSSHSignature bool
}

// StartServicesDaemon starts the services daemon for credential forwarding and port forwarding
func StartServicesDaemon(ctx context.Context, opts StartServicesDaemonOptions) error {
	if opts.User == "" {
		return nil
	}

	workspace, err := getWorkspace(ctx, opts.Client)
	if err != nil {
		return err
	}

	credConfig := getCredentialConfig(opts.DevPodConfig, workspace)

	return tunnel.RunServices(
		ctx,
		opts.DevPodConfig,
		opts.SSHClient,
		opts.User,
		opts.ForwardPorts,
		opts.ExtraPorts,
		nil,
		opts.Client.WorkspaceConfig(),
		credConfig.docker,
		credConfig.git,
		credConfig.gitSSHSignature,
		opts.Log,
	)
}

func getWorkspace(ctx context.Context, client client.DaemonClient) (*managementv1.DevPodWorkspaceInstance, error) {
	return daemon.NewLocalClient(client.Provider()).GetWorkspace(ctx, client.WorkspaceConfig().UID)
}

func getCredentialConfig(devPodConfig *config.Config, workspace *managementv1.DevPodWorkspaceInstance) credentialConfig {
	cfg := credentialConfig{
		docker:          devPodConfig.ContextOption(config.ContextOptionSSHInjectDockerCredentials) == "true",
		git:             devPodConfig.ContextOption(config.ContextOptionSSHInjectGitCredentials) == "true",
		gitSSHSignature: devPodConfig.ContextOption(config.ContextOptionGitSSHSignatureForwarding) == "true",
	}

	if workspace == nil || workspace.Status.Instance == nil {
		return cfg
	}

	instance := workspace.Status.Instance
	if instance.CredentialForwarding == nil {
		return cfg
	}

	instanceCredentialForwarding := instance.CredentialForwarding
	if instanceCredentialForwarding.Docker != nil {
		cfg.docker = !instanceCredentialForwarding.Docker.Disabled
	}
	if instanceCredentialForwarding.Git != nil {
		cfg.git = !instanceCredentialForwarding.Git.Disabled
		cfg.gitSSHSignature = !instanceCredentialForwarding.Git.Disabled
	}

	return cfg
}
