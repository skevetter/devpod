package opener

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
	client2 "github.com/skevetter/devpod/pkg/client"
	"github.com/skevetter/devpod/pkg/client/clientimplementation"
	"github.com/skevetter/devpod/pkg/command"
	"github.com/skevetter/devpod/pkg/config"
	config2 "github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/skevetter/devpod/pkg/gpg"
	"github.com/skevetter/devpod/pkg/ide/fleet"
	"github.com/skevetter/devpod/pkg/ide/jetbrains"
	"github.com/skevetter/devpod/pkg/ide/jupyter"
	"github.com/skevetter/devpod/pkg/ide/openvscode"
	"github.com/skevetter/devpod/pkg/ide/rstudio"
	"github.com/skevetter/devpod/pkg/ide/vscode"
	"github.com/skevetter/devpod/pkg/ide/zed"
	open2 "github.com/skevetter/devpod/pkg/open"
	"github.com/skevetter/devpod/pkg/port"
	devssh "github.com/skevetter/devpod/pkg/ssh"
	"github.com/skevetter/devpod/pkg/tunnel"
	"github.com/skevetter/log"
	"github.com/skratchdot/open-golang/open"
	"golang.org/x/crypto/ssh"
)

// Params holds the parameters needed to open an IDE.
type Params struct {
	GPGAgentForwarding bool
	SSHAuthSockID      string
	GitSSHSigningKey   string
	DevPodConfig       *config.Config
	Client             client2.BaseWorkspaceClient
	User               string
	Result             *config2.Result
	Log                log.Logger
}

// Open dispatches to the correct IDE opener based on ideName.
func Open(
	ctx context.Context,
	ideName string,
	ideOptions map[string]config.OptionValue,
	params Params,
) error {
	if fn, ok := browserIDEOpener(ideName); ok {
		return fn(ctx, ideOptions, params)
	}

	return openDesktopIDE(ctx, ideName, ideOptions, params)
}

// browserIDEOpener returns a handler for browser-based IDEs if ideName matches.
func browserIDEOpener(
	ideName string,
) (func(context.Context, map[string]config.OptionValue, Params) error, bool) {
	switch ideName {
	case string(config.IDEOpenVSCode):
		return openVSCodeBrowser, true
	case string(config.IDEJupyterNotebook):
		return openJupyterBrowser, true
	case string(config.IDERStudio):
		return openRStudioBrowser, true
	default:
		return nil, false
	}
}

func openDesktopIDE(
	ctx context.Context,
	ideName string,
	ideOptions map[string]config.OptionValue,
	params Params,
) error {
	switch ideName {
	case string(config.IDEVSCode), string(config.IDEVSCodeInsiders), string(config.IDECursor),
		string(config.IDECodium), string(config.IDEPositron), string(config.IDEWindsurf),
		string(config.IDEAntigravity), string(config.IDEBob):
		return openVSCodeFlavor(ctx, ideName, ideOptions, params)

	case string(config.IDERustRover), string(config.IDEGoland), string(config.IDEPyCharm),
		string(config.IDEPhpStorm), string(config.IDEIntellij), string(config.IDECLion),
		string(config.IDERider), string(config.IDERubyMine), string(config.IDEWebStorm),
		string(config.IDEDataSpell):
		return openJetBrains(ideName, ideOptions, params)

	case string(config.IDEFleet):
		return startFleet(ctx, params.Client, params.Log)

	case string(config.IDEZed):
		return zed.Open(
			ctx, ideOptions, params.User,
			params.Result.SubstitutionContext.ContainerWorkspaceFolder,
			params.Client.Workspace(), params.Log,
		)

	default:
		return nil
	}
}

// ParseAddressAndPort parses a bind address option into host address and port.
// If bindAddressOption is empty, it finds an available port starting from defaultPort.
func ParseAddressAndPort(bindAddressOption string, defaultPort int) (string, int, error) {
	if bindAddressOption == "" {
		return parseDefaultPort(defaultPort)
	}

	return parseExplicitAddress(bindAddressOption)
}

func parseDefaultPort(defaultPort int) (string, int, error) {
	portName, err := port.FindAvailablePort(defaultPort)
	if err != nil {
		return "", 0, err
	}

	return fmt.Sprintf("%d", portName), portName, nil
}

func parseExplicitAddress(address string) (string, int, error) {
	_, p, err := net.SplitHostPort(address)
	if err != nil {
		return "", 0, fmt.Errorf("parse host:port: %w", err)
	}
	if p == "" {
		return "", 0, fmt.Errorf("parse ADDRESS: expected host:port, got %s", address)
	}

	portName, err := strconv.Atoi(p)
	if err != nil {
		return "", 0, fmt.Errorf("parse host:port: %w", err)
	}

	return address, portName, nil
}

var vsCodeFlavorMap = map[string]vscode.Flavor{
	string(config.IDEVSCode):         vscode.FlavorStable,
	string(config.IDEVSCodeInsiders): vscode.FlavorInsiders,
	string(config.IDECursor):         vscode.FlavorCursor,
	string(config.IDECodium):         vscode.FlavorCodium,
	string(config.IDEPositron):       vscode.FlavorPositron,
	string(config.IDEWindsurf):       vscode.FlavorWindsurf,
	string(config.IDEAntigravity):    vscode.FlavorAntigravity,
	string(config.IDEBob):            vscode.FlavorBob,
}

func openVSCodeFlavor(
	ctx context.Context,
	ideName string,
	ideOptions map[string]config.OptionValue,
	params Params,
) error {
	return vscode.Open(ctx, vscode.OpenParams{
		Workspace: params.Client.Workspace(),
		Folder:    params.Result.SubstitutionContext.ContainerWorkspaceFolder,
		NewWindow: vscode.Options.GetValue(ideOptions, vscode.OpenNewWindow) == config.BoolTrue,
		Flavor:    vsCodeFlavorMap[ideName],
		Log:       params.Log,
	})
}

func openJetBrains(
	ideName string,
	ideOptions map[string]config.OptionValue,
	params Params,
) error {
	folder := params.Result.SubstitutionContext.ContainerWorkspaceFolder
	workspace := params.Client.Workspace()
	user := params.User
	logger := params.Log
	type jetbrainsFactory func() interface{ OpenGateway(string, string) error }

	jetbrainsMap := map[string]jetbrainsFactory{
		string(config.IDERustRover): func() interface{ OpenGateway(string, string) error } {
			return jetbrains.NewRustRoverServer(user, ideOptions, logger)
		},
		string(config.IDEGoland): func() interface{ OpenGateway(string, string) error } {
			return jetbrains.NewGolandServer(user, ideOptions, logger)
		},
		string(config.IDEPyCharm): func() interface{ OpenGateway(string, string) error } {
			return jetbrains.NewPyCharmServer(user, ideOptions, logger)
		},
		string(config.IDEPhpStorm): func() interface{ OpenGateway(string, string) error } {
			return jetbrains.NewPhpStorm(user, ideOptions, logger)
		},
		string(config.IDEIntellij): func() interface{ OpenGateway(string, string) error } {
			return jetbrains.NewIntellij(user, ideOptions, logger)
		},
		string(config.IDECLion): func() interface{ OpenGateway(string, string) error } {
			return jetbrains.NewCLionServer(user, ideOptions, logger)
		},
		string(config.IDERider): func() interface{ OpenGateway(string, string) error } {
			return jetbrains.NewRiderServer(user, ideOptions, logger)
		},
		string(config.IDERubyMine): func() interface{ OpenGateway(string, string) error } {
			return jetbrains.NewRubyMineServer(user, ideOptions, logger)
		},
		string(config.IDEWebStorm): func() interface{ OpenGateway(string, string) error } {
			return jetbrains.NewWebStormServer(user, ideOptions, logger)
		},
		string(config.IDEDataSpell): func() interface{ OpenGateway(string, string) error } {
			return jetbrains.NewDataSpellServer(user, ideOptions, logger)
		},
	}

	if factory, ok := jetbrainsMap[ideName]; ok {
		return factory().OpenGateway(folder, workspace)
	}
	return fmt.Errorf("unknown JetBrains IDE: %s", ideName)
}

// browserTunnelParams bundles the arguments for browser-based IDE tunnels.
type browserTunnelParams struct {
	ctx              context.Context
	devPodConfig     *config.Config
	client           client2.BaseWorkspaceClient
	user             string
	targetURL        string
	forwardPorts     bool
	extraPorts       []string
	authSockID       string
	gitSSHSigningKey string
	logger           log.Logger
}

func openJupyterBrowser(
	ctx context.Context,
	ideOptions map[string]config.OptionValue,
	params Params,
) error {
	if params.GPGAgentForwarding {
		if err := gpg.ForwardAgent(params.Client, params.Log); err != nil {
			return err
		}
	}

	addr, jupyterPort, err := ParseAddressAndPort(
		jupyter.Options.GetValue(ideOptions, jupyter.BindAddressOption),
		jupyter.DefaultServerPort,
	)
	if err != nil {
		return err
	}

	targetURL := fmt.Sprintf("http://localhost:%d/lab", jupyterPort)
	if jupyter.Options.GetValue(ideOptions, jupyter.OpenOption) == config.BoolTrue {
		go func() {
			if openErr := open2.Open(ctx, targetURL, params.Log); openErr != nil {
				params.Log.WithFields(logrus.Fields{"error": openErr}).
					Error("error opening jupyter notebook")
			}

			params.Log.Info(
				"started jupyter notebook in browser mode. " +
					"Please keep this terminal open as long as you use Jupyter Notebook",
			)
		}()
	}

	params.Log.Infof("Starting jupyter notebook in browser mode at %s", targetURL)
	return startBrowserTunnel(browserTunnelParams{
		ctx:              ctx,
		devPodConfig:     params.DevPodConfig,
		client:           params.Client,
		user:             params.User,
		targetURL:        targetURL,
		extraPorts:       []string{fmt.Sprintf("%s:%d", addr, jupyter.DefaultServerPort)},
		authSockID:       params.SSHAuthSockID,
		gitSSHSigningKey: params.GitSSHSigningKey,
		logger:           params.Log,
	})
}

func openRStudioBrowser(
	ctx context.Context,
	ideOptions map[string]config.OptionValue,
	params Params,
) error {
	if params.GPGAgentForwarding {
		if err := gpg.ForwardAgent(params.Client, params.Log); err != nil {
			return err
		}
	}

	addr, rsPort, err := ParseAddressAndPort(
		rstudio.Options.GetValue(ideOptions, rstudio.BindAddressOption),
		rstudio.DefaultServerPort,
	)
	if err != nil {
		return err
	}

	targetURL := fmt.Sprintf("http://localhost:%d", rsPort)
	if rstudio.Options.GetValue(ideOptions, rstudio.OpenOption) == config.BoolTrue {
		go func() {
			if openErr := open2.Open(ctx, targetURL, params.Log); openErr != nil {
				params.Log.Errorf("error opening rstudio: %v", openErr)
			}

			params.Log.Infof(
				"started RStudio Server in browser mode. Please keep this terminal open as long as you use it",
			)
		}()
	}

	params.Log.Infof("Starting RStudio server in browser mode at %s", targetURL)
	return startBrowserTunnel(browserTunnelParams{
		ctx:              ctx,
		devPodConfig:     params.DevPodConfig,
		client:           params.Client,
		user:             params.User,
		targetURL:        targetURL,
		extraPorts:       []string{fmt.Sprintf("%s:%d", addr, rstudio.DefaultServerPort)},
		authSockID:       params.SSHAuthSockID,
		gitSSHSigningKey: params.GitSSHSigningKey,
		logger:           params.Log,
	})
}

func openVSCodeBrowser(
	ctx context.Context,
	ideOptions map[string]config.OptionValue,
	params Params,
) error {
	if params.GPGAgentForwarding {
		if err := gpg.ForwardAgent(params.Client, params.Log); err != nil {
			return err
		}
	}

	folder := params.Result.SubstitutionContext.ContainerWorkspaceFolder
	addr, vscodePort, err := ParseAddressAndPort(
		openvscode.Options.GetValue(ideOptions, openvscode.BindAddressOption),
		openvscode.DefaultVSCodePort,
	)
	if err != nil {
		return err
	}

	targetURL := fmt.Sprintf("http://localhost:%d/?folder=%s", vscodePort, folder)
	if openvscode.Options.GetValue(ideOptions, openvscode.OpenOption) == config.BoolTrue {
		go func() {
			if openErr := open2.Open(ctx, targetURL, params.Log); openErr != nil {
				params.Log.Errorf("error opening vscode: %v", openErr)
			}

			params.Log.Infof(
				"started vscode in browser mode. " +
					"Please keep this terminal open as long as you use VSCode browser version",
			)
		}()
	}

	params.Log.Infof("Starting vscode in browser mode at %s", targetURL)
	forwardPorts := openvscode.Options.GetValue(
		ideOptions,
		openvscode.ForwardPortsOption,
	) == config.BoolTrue
	return startBrowserTunnel(browserTunnelParams{
		ctx:              ctx,
		devPodConfig:     params.DevPodConfig,
		client:           params.Client,
		user:             params.User,
		targetURL:        targetURL,
		forwardPorts:     forwardPorts,
		extraPorts:       []string{fmt.Sprintf("%s:%d", addr, openvscode.DefaultVSCodePort)},
		authSockID:       params.SSHAuthSockID,
		gitSSHSigningKey: params.GitSSHSigningKey,
		logger:           params.Log,
	})
}

func startFleet(ctx context.Context, client client2.BaseWorkspaceClient, logger log.Logger) error {
	stdout := &bytes.Buffer{}
	sshCmd, err := createSSHCommand(
		ctx,
		client,
		logger,
		[]string{"--command", "cat " + fleet.FleetURLFileName},
	)
	if err != nil {
		return err
	}
	sshCmd.Stdout = stdout
	err = sshCmd.Run()
	if err != nil {
		return command.WrapCommandError(stdout.Bytes(), err)
	}

	url := strings.TrimSpace(stdout.String())
	if len(url) == 0 {
		return fmt.Errorf("seems like fleet is not running within the container")
	}

	logger.Warnf(
		"Fleet is exposed at a publicly reachable URL, please make sure to not disclose this URL " +
			"to anyone as they will be able to reach your workspace from that",
	)
	logger.Infof("Starting Fleet at %s ...", url)

	return open.Run(url)
}

func setupBackhaul(client client2.BaseWorkspaceClient, authSockID string, logger log.Logger) error {
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
	dotCmd := exec.Command(
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
		"while true; do sleep 6000000; done",
	)

	if logger.GetLevel() == logrus.DebugLevel {
		dotCmd.Args = append(dotCmd.Args, "--debug")
	}

	logger.Info("Setting up backhaul SSH connection")

	writer := logger.Writer(logrus.InfoLevel, false)

	dotCmd.Stdout = writer
	dotCmd.Stderr = writer

	err = dotCmd.Run()
	if err != nil {
		return err
	}

	logger.Infof("Done setting up backhaul")

	return nil
}

func createSSHCommand(
	ctx context.Context,
	client client2.BaseWorkspaceClient,
	logger log.Logger,
	extraArgs []string,
) (*exec.Cmd, error) {
	execPath, err := os.Executable()
	if err != nil {
		return nil, err
	}

	args := []string{
		"ssh",
		"--user=root",
		"--agent-forwarding=false",
		"--start-services=false",
		"--context",
		client.Context(),
		client.Workspace(),
	}
	if logger.GetLevel() == logrus.DebugLevel {
		args = append(args, "--debug")
	}
	args = append(args, extraArgs...)

	//nolint:gosec // execPath is the current binary, arguments are controlled
	return exec.CommandContext(ctx, execPath, args...), nil
}

func startBrowserTunnel(p browserTunnelParams) error {
	if p.authSockID != "" {
		go func() {
			if err := setupBackhaul(p.client, p.authSockID, p.logger); err != nil {
				p.logger.Error("Failed to setup backhaul SSH connection: ", err)
			}
		}()
	}

	daemonClient, ok := p.client.(client2.DaemonClient)
	if ok {
		return startBrowserTunnelDaemon(p, daemonClient)
	}

	return startBrowserTunnelSSH(p)
}

func startBrowserTunnelDaemon(p browserTunnelParams, daemonClient client2.DaemonClient) error {
	toolClient, _, err := daemonClient.SSHClients(p.ctx, p.user)
	if err != nil {
		return err
	}
	defer func() { _ = toolClient.Close() }()

	err = clientimplementation.StartServicesDaemon(
		p.ctx,
		clientimplementation.StartServicesDaemonOptions{
			DevPodConfig: p.devPodConfig,
			Client:       daemonClient,
			SSHClient:    toolClient,
			User:         p.user,
			Log:          p.logger,
			ForwardPorts: p.forwardPorts,
			ExtraPorts:   p.extraPorts,
		},
	)
	if err != nil {
		return err
	}
	<-p.ctx.Done()

	return nil
}

func startBrowserTunnelSSH(p browserTunnelParams) error {
	return tunnel.NewTunnel(
		p.ctx,
		func(ctx context.Context, stdin io.Reader, stdout io.Writer) error {
			writer := p.logger.Writer(logrus.DebugLevel, false)
			defer func() { _ = writer.Close() }()

			sshCmd, err := createSSHCommand(ctx, p.client, p.logger, []string{
				"--log-output=raw",
				fmt.Sprintf("--reuse-ssh-auth-sock=%s", p.authSockID),
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
	p browserTunnelParams,
	containerClient *ssh.Client,
) error {
	streamLogger, ok := p.logger.(*log.StreamLogger)
	if ok {
		streamLogger.JSON(logrus.InfoLevel, map[string]string{
			"url":  p.targetURL,
			"done": "true",
		})
	}

	err := tunnel.RunServices(
		ctx,
		tunnel.RunServicesOptions{
			DevPodConfig:    p.devPodConfig,
			ContainerClient: containerClient,
			User:            p.user,
			ForwardPorts:    p.forwardPorts,
			ExtraPorts:      p.extraPorts,
			Workspace:       p.client.WorkspaceConfig(),
			ConfigureDockerCredentials: p.devPodConfig.ContextOption(
				config.ContextOptionSSHInjectDockerCredentials,
			) == config.BoolTrue,
			ConfigureGitCredentials: p.devPodConfig.ContextOption(
				config.ContextOptionSSHInjectGitCredentials,
			) == config.BoolTrue,
			ConfigureGitSSHSignatureHelper: p.devPodConfig.ContextOption(
				config.ContextOptionGitSSHSignatureForwarding,
			) == config.BoolTrue,
			GitSSHSigningKey: p.gitSSHSigningKey,
			Log:              p.logger,
		},
	)
	if err != nil {
		return fmt.Errorf("run credentials server in browser tunnel: %w", err)
	}

	<-ctx.Done()
	return nil
}
