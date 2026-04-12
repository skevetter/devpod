package tunnel

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/sirupsen/logrus"
	client2 "github.com/skevetter/devpod/pkg/client"
	"github.com/skevetter/devpod/pkg/config"
	devssh "github.com/skevetter/devpod/pkg/ssh"
	"github.com/skevetter/log"
	"golang.org/x/crypto/ssh"
)

// BrowserTunnelParams bundles the arguments for browser-based IDE tunnels.
type BrowserTunnelParams struct {
	Ctx              context.Context
	DevPodConfig     *config.Config
	Client           client2.BaseWorkspaceClient
	User             string
	TargetURL        string
	ForwardPorts     bool
	ExtraPorts       []string
	AuthSockID       string
	GitSSHSigningKey string
	Logger           log.Logger

	// DaemonStartFunc is called when the client is a DaemonClient.
	// If nil, the SSH tunnel path is always used.
	DaemonStartFunc func(ctx context.Context) error
}

// StartBrowserTunnel sets up a browser tunnel for IDE access, either via daemon or SSH.
func StartBrowserTunnel(p BrowserTunnelParams) error {
	if p.AuthSockID != "" {
		go func() {
			if err := SetupBackhaul(p.Ctx, p.Client, p.AuthSockID, p.Logger); err != nil {
				p.Logger.Error("Failed to setup backhaul SSH connection: ", err)
			}
		}()
	}

	if p.DaemonStartFunc != nil {
		return p.DaemonStartFunc(p.Ctx)
	}

	return startBrowserTunnelSSH(p)
}

func startBrowserTunnelSSH(p BrowserTunnelParams) error {
	return NewTunnel(
		p.Ctx,
		func(ctx context.Context, stdin io.Reader, stdout io.Writer) error {
			writer := p.Logger.Writer(logrus.DebugLevel, false)
			defer func() { _ = writer.Close() }()

			sshCmd, err := CreateSSHCommand(ctx, p.Client, p.Logger, []string{
				"--log-output=raw",
				fmt.Sprintf("--reuse-ssh-auth-sock=%s", p.AuthSockID),
				"--stdio",
			})
			if err != nil {
				return err
			}
			sshCmd.Stdout = stdout
			sshCmd.Stdin = stdin
			sshCmd.Stderr = writer
			return sshCmd.Run()
		},
		func(ctx context.Context, containerClient *ssh.Client) error {
			return runBrowserTunnelServices(ctx, p, containerClient)
		},
	)
}

func runBrowserTunnelServices(
	ctx context.Context,
	p BrowserTunnelParams,
	containerClient *ssh.Client,
) error {
	streamLogger, ok := p.Logger.(*log.StreamLogger)
	if ok {
		streamLogger.JSON(logrus.InfoLevel, map[string]string{
			"url":  p.TargetURL,
			"done": "true",
		})
	}

	err := RunServices(
		ctx,
		RunServicesOptions{
			DevPodConfig:    p.DevPodConfig,
			ContainerClient: containerClient,
			User:            p.User,
			ForwardPorts:    p.ForwardPorts,
			ExtraPorts:      p.ExtraPorts,
			Workspace:       p.Client.WorkspaceConfig(),
			ConfigureDockerCredentials: p.DevPodConfig.ContextOption(
				config.ContextOptionSSHInjectDockerCredentials,
			) == config.BoolTrue,
			ConfigureGitCredentials: p.DevPodConfig.ContextOption(
				config.ContextOptionSSHInjectGitCredentials,
			) == config.BoolTrue,
			ConfigureGitSSHSignatureHelper: p.DevPodConfig.ContextOption(
				config.ContextOptionGitSSHSignatureForwarding,
			) == config.BoolTrue,
			GitSSHSigningKey: p.GitSSHSigningKey,
			Log:              p.Logger,
		},
	)
	if err != nil {
		return fmt.Errorf("run credentials server in browser tunnel: %w", err)
	}

	<-ctx.Done()
	return nil
}

// SetupBackhaul sets up a long-running SSH connection for backhaul.
func SetupBackhaul(
	ctx context.Context,
	client client2.BaseWorkspaceClient,
	authSockID string,
	logger log.Logger,
) error {
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

	//nolint:gosec // execPath is the current binary, arguments are controlled
	backhaulCmd := exec.CommandContext(ctx,
		execPath,
		"ssh",
		"--agent-forwarding=true",
		fmt.Sprintf("--reuse-ssh-auth-sock=%s", authSockID),
		"--start-services=false",
		"--user",
		remoteUser,
		"--context",
		client.Context(),
		client.Workspace(),
		"--log-output=raw",
		"--command",
		"while true; do sleep 6000000; done", // sleep infinity is not available on all systems
	)

	if logger.GetLevel() == logrus.DebugLevel {
		backhaulCmd.Args = append(backhaulCmd.Args, "--debug")
	}

	logger.Info("Setting up backhaul SSH connection")

	writer := logger.Writer(logrus.InfoLevel, false)
	defer func() { _ = writer.Close() }()

	backhaulCmd.Stdout = writer
	backhaulCmd.Stderr = writer

	err = backhaulCmd.Run()
	if err != nil {
		return err
	}

	logger.Infof("Done setting up backhaul")

	return nil
}

// CreateSSHCommand builds an exec.Cmd that runs `devpod ssh` with the given arguments.
func CreateSSHCommand(
	ctx context.Context,
	client client2.BaseWorkspaceClient,
	logger log.Logger,
	extraArgs []string,
) (*exec.Cmd, error) {
	execPath, err := os.Executable()
	if err != nil {
		return nil, err
	}

	args := buildSSHCommandArgs(
		client.Context(),
		client.Workspace(),
		logger.GetLevel() == logrus.DebugLevel,
		extraArgs,
	)

	//nolint:gosec // execPath is the current binary, arguments are controlled
	return exec.CommandContext(ctx, execPath, args...), nil
}

// buildSSHCommandArgs constructs the argument list for `devpod ssh`.
func buildSSHCommandArgs(clientContext, workspace string, debug bool, extraArgs []string) []string {
	args := []string{
		"ssh",
		"--user=root",
		"--agent-forwarding=false",
		"--start-services=false",
		"--context",
		clientContext,
		workspace,
	}
	if debug {
		args = append(args, "--debug")
	}
	args = append(args, extraArgs...)
	return args
}
