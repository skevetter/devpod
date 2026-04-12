package opener

import (
	"bytes"
	"context"
	"fmt"
	"net"
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
	"github.com/skevetter/devpod/pkg/tunnel"
	"github.com/skevetter/log"
	"github.com/skratchdot/open-golang/open"
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

func makeDaemonStartFunc(
	params Params,
	forwardPorts bool,
	extraPorts []string,
) func(ctx context.Context) error {
	daemonClient, ok := params.Client.(client2.DaemonClient)
	if !ok {
		return nil
	}

	return func(ctx context.Context) error {
		toolClient, _, err := daemonClient.SSHClients(ctx, params.User)
		if err != nil {
			return err
		}
		defer func() { _ = toolClient.Close() }()

		err = clientimplementation.StartServicesDaemon(
			ctx,
			clientimplementation.StartServicesDaemonOptions{
				DevPodConfig: params.DevPodConfig,
				Client:       daemonClient,
				SSHClient:    toolClient,
				User:         params.User,
				Log:          params.Log,
				ForwardPorts: forwardPorts,
				ExtraPorts:   extraPorts,
			},
		)
		if err != nil {
			return err
		}
		<-ctx.Done()

		return nil
	}
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
	extraPorts := []string{fmt.Sprintf("%s:%d", addr, jupyter.DefaultServerPort)}
	return tunnel.StartBrowserTunnel(tunnel.BrowserTunnelParams{
		Ctx:              ctx,
		DevPodConfig:     params.DevPodConfig,
		Client:           params.Client,
		User:             params.User,
		TargetURL:        targetURL,
		ExtraPorts:       extraPorts,
		AuthSockID:       params.SSHAuthSockID,
		GitSSHSigningKey: params.GitSSHSigningKey,
		Logger:           params.Log,
		DaemonStartFunc:  makeDaemonStartFunc(params, false, extraPorts),
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
	extraPorts := []string{fmt.Sprintf("%s:%d", addr, rstudio.DefaultServerPort)}
	return tunnel.StartBrowserTunnel(tunnel.BrowserTunnelParams{
		Ctx:              ctx,
		DevPodConfig:     params.DevPodConfig,
		Client:           params.Client,
		User:             params.User,
		TargetURL:        targetURL,
		ExtraPorts:       extraPorts,
		AuthSockID:       params.SSHAuthSockID,
		GitSSHSigningKey: params.GitSSHSigningKey,
		Logger:           params.Log,
		DaemonStartFunc:  makeDaemonStartFunc(params, false, extraPorts),
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
	extraPorts := []string{fmt.Sprintf("%s:%d", addr, openvscode.DefaultVSCodePort)}
	return tunnel.StartBrowserTunnel(tunnel.BrowserTunnelParams{
		Ctx:              ctx,
		DevPodConfig:     params.DevPodConfig,
		Client:           params.Client,
		User:             params.User,
		TargetURL:        targetURL,
		ForwardPorts:     forwardPorts,
		ExtraPorts:       extraPorts,
		AuthSockID:       params.SSHAuthSockID,
		GitSSHSigningKey: params.GitSSHSigningKey,
		Logger:           params.Log,
		DaemonStartFunc:  makeDaemonStartFunc(params, forwardPorts, extraPorts),
	})
}

func startFleet(ctx context.Context, client client2.BaseWorkspaceClient, logger log.Logger) error {
	stdout := &bytes.Buffer{}
	sshCmd, err := tunnel.CreateSSHCommand(
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
