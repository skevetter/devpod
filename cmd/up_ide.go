package cmd

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
	"github.com/skevetter/devpod/pkg/agent"
	"github.com/skevetter/devpod/pkg/agent/tunnelserver"
	client2 "github.com/skevetter/devpod/pkg/client"
	"github.com/skevetter/devpod/pkg/client/clientimplementation"
	"github.com/skevetter/devpod/pkg/command"
	"github.com/skevetter/devpod/pkg/config"
	config2 "github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/skevetter/devpod/pkg/devcontainer/sshtunnel"
	"github.com/skevetter/devpod/pkg/ide/fleet"
	"github.com/skevetter/devpod/pkg/ide/jetbrains"
	"github.com/skevetter/devpod/pkg/ide/jupyter"
	"github.com/skevetter/devpod/pkg/ide/openvscode"
	"github.com/skevetter/devpod/pkg/ide/rstudio"
	"github.com/skevetter/devpod/pkg/ide/vscode"
	"github.com/skevetter/devpod/pkg/ide/zed"
	open2 "github.com/skevetter/devpod/pkg/open"
	"github.com/skevetter/devpod/pkg/port"
	provider2 "github.com/skevetter/devpod/pkg/provider"
	devssh "github.com/skevetter/devpod/pkg/ssh"
	"github.com/skevetter/devpod/pkg/tunnel"
	"github.com/skevetter/log"
	"github.com/skratchdot/open-golang/open"
	"golang.org/x/crypto/ssh"
)

func (cmd *UpCmd) openIDE(ctx context.Context, devPodConfig *config.Config, client client2.BaseWorkspaceClient, wctx *workspaceContext, log log.Logger) error {
	if !cmd.OpenIDE {
		return nil
	}

	ideConfig := client.WorkspaceConfig().IDE
	opener := newIDEOpener(cmd, devPodConfig, client, wctx, log)
	return opener.open(ctx, ideConfig.Name, ideConfig.Options)
}

// ideOpener handles opening different IDE types
type ideOpener struct {
	cmd          *UpCmd
	devPodConfig *config.Config
	client       client2.BaseWorkspaceClient
	wctx         *workspaceContext
	log          log.Logger
}

func newIDEOpener(cmd *UpCmd, devPodConfig *config.Config, client client2.BaseWorkspaceClient, wctx *workspaceContext, log log.Logger) *ideOpener {
	return &ideOpener{
		cmd:          cmd,
		devPodConfig: devPodConfig,
		client:       client,
		wctx:         wctx,
		log:          log,
	}
}

func (o *ideOpener) open(ctx context.Context, ideName string, ideOptions map[string]config.OptionValue) error {
	folder := o.wctx.result.SubstitutionContext.ContainerWorkspaceFolder
	workspace := o.client.Workspace()
	user := o.wctx.user

	switch ideName {
	case string(config.IDEVSCode), string(config.IDEVSCodeInsiders), string(config.IDECursor),
		string(config.IDECodium), string(config.IDEPositron), string(config.IDEWindsurf), string(config.IDEAntigravity):
		return o.openVSCodeFlavor(ctx, ideName, folder, ideOptions)

	case string(config.IDERustRover), string(config.IDEGoland), string(config.IDEPyCharm),
		string(config.IDEPhpStorm), string(config.IDEIntellij), string(config.IDECLion),
		string(config.IDERider), string(config.IDERubyMine), string(config.IDEWebStorm), string(config.IDEDataSpell):
		return o.openJetBrains(ideName, folder, workspace, user, ideOptions)

	case string(config.IDEOpenVSCode):
		return startVSCodeInBrowser(o.cmd.GPGAgentForwarding, ctx, o.devPodConfig, o.client, folder, user, ideOptions, o.cmd.SSHAuthSockID, o.log)

	case string(config.IDEFleet):
		return startFleet(ctx, o.client, o.log)

	case string(config.IDEZed):
		return zed.Open(ctx, ideOptions, user, folder, workspace, o.log)

	case string(config.IDEJupyterNotebook):
		return startJupyterNotebookInBrowser(o.cmd.GPGAgentForwarding, ctx, o.devPodConfig, o.client, user, ideOptions, o.cmd.SSHAuthSockID, o.log)

	case string(config.IDERStudio):
		return startRStudioInBrowser(o.cmd.GPGAgentForwarding, ctx, o.devPodConfig, o.client, user, ideOptions, o.cmd.SSHAuthSockID, o.log)

	default:
		return nil
	}
}

func (o *ideOpener) openVSCodeFlavor(ctx context.Context, ideName, folder string, ideOptions map[string]config.OptionValue) error {
	flavorMap := map[string]vscode.Flavor{
		string(config.IDEVSCode):         vscode.FlavorStable,
		string(config.IDEVSCodeInsiders): vscode.FlavorInsiders,
		string(config.IDECursor):         vscode.FlavorCursor,
		string(config.IDECodium):         vscode.FlavorCodium,
		string(config.IDEPositron):       vscode.FlavorPositron,
		string(config.IDEWindsurf):       vscode.FlavorWindsurf,
		string(config.IDEAntigravity):    vscode.FlavorAntigravity,
	}

	params := vscode.OpenParams{
		Workspace: o.client.Workspace(),
		Folder:    folder,
		NewWindow: vscode.Options.GetValue(ideOptions, vscode.OpenNewWindow) == "true",
		Flavor:    flavorMap[ideName],
		Log:       o.log,
	}

	return vscode.Open(ctx, params)
}

func (o *ideOpener) openJetBrains(ideName, folder, workspace, user string, ideOptions map[string]config.OptionValue) error {
	type jetbrainsFactory func() interface{ OpenGateway(string, string) error }

	jetbrainsMap := map[string]jetbrainsFactory{
		string(config.IDERustRover): func() interface{ OpenGateway(string, string) error } {
			return jetbrains.NewRustRoverServer(user, ideOptions, o.log)
		},
		string(config.IDEGoland): func() interface{ OpenGateway(string, string) error } {
			return jetbrains.NewGolandServer(user, ideOptions, o.log)
		},
		string(config.IDEPyCharm): func() interface{ OpenGateway(string, string) error } {
			return jetbrains.NewPyCharmServer(user, ideOptions, o.log)
		},
		string(config.IDEPhpStorm): func() interface{ OpenGateway(string, string) error } {
			return jetbrains.NewPhpStorm(user, ideOptions, o.log)
		},
		string(config.IDEIntellij): func() interface{ OpenGateway(string, string) error } {
			return jetbrains.NewIntellij(user, ideOptions, o.log)
		},
		string(config.IDECLion): func() interface{ OpenGateway(string, string) error } {
			return jetbrains.NewCLionServer(user, ideOptions, o.log)
		},
		string(config.IDERider): func() interface{ OpenGateway(string, string) error } {
			return jetbrains.NewRiderServer(user, ideOptions, o.log)
		},
		string(config.IDERubyMine): func() interface{ OpenGateway(string, string) error } {
			return jetbrains.NewRubyMineServer(user, ideOptions, o.log)
		},
		string(config.IDEWebStorm): func() interface{ OpenGateway(string, string) error } {
			return jetbrains.NewWebStormServer(user, ideOptions, o.log)
		},
		string(config.IDEDataSpell): func() interface{ OpenGateway(string, string) error } {
			return jetbrains.NewDataSpellServer(user, ideOptions, o.log)
		},
	}

	if factory, ok := jetbrainsMap[ideName]; ok {
		return factory().OpenGateway(folder, workspace)
	}
	return fmt.Errorf("unknown JetBrains IDE: %s", ideName)
}

func (cmd *UpCmd) devPodUp(
	ctx context.Context,
	devPodConfig *config.Config,
	client client2.BaseWorkspaceClient,
	log log.Logger,
) (*config2.Result, error) {
	var err error

	// only lock if we are not in platform mode
	if !cmd.Platform.Enabled {
		err := client.Lock(ctx)
		if err != nil {
			return nil, err
		}
		defer client.Unlock()
	}

	// get result
	var result *config2.Result

	switch client := client.(type) {
	case client2.WorkspaceClient:
		result, err = cmd.devPodUpMachine(ctx, devPodConfig, client, log)
		if err != nil {
			return nil, err
		}
	case client2.ProxyClient:
		result, err = cmd.devPodUpProxy(ctx, client, log)
		if err != nil {
			return nil, err
		}
	case client2.DaemonClient:
		result, err = cmd.devPodUpDaemon(ctx, client)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported client type: %T", client)
	}

	// save result to file
	err = provider2.SaveWorkspaceResult(client.WorkspaceConfig(), result)
	if err != nil {
		return nil, fmt.Errorf("save workspace result: %w", err)
	}

	return result, nil
}

func (cmd *UpCmd) devPodUpProxy(
	ctx context.Context,
	client client2.ProxyClient,
	log log.Logger,
) (*config2.Result, error) {
	// create pipes
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	stdinReader, stdinWriter, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer func() { _ = stdoutWriter.Close() }()
	defer func() { _ = stdinWriter.Close() }()

	// start machine on stdio
	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// create up command
	errChan := make(chan error, 1)
	go func() {
		defer log.Debug("done executing up command")
		defer cancel()

		// build devpod up options
		workspace := client.WorkspaceConfig()
		baseOptions := cmd.CLIOptions
		baseOptions.ID = workspace.ID
		baseOptions.DevContainerPath = workspace.DevContainerPath
		baseOptions.DevContainerImage = workspace.DevContainerImage
		baseOptions.IDE = workspace.IDE.Name
		baseOptions.IDEOptions = nil
		baseOptions.Source = workspace.Source.String()
		for optionName, optionValue := range workspace.IDE.Options {
			baseOptions.IDEOptions = append(
				baseOptions.IDEOptions,
				optionName+"="+optionValue.Value,
			)
		}

		// run devpod up elsewhere
		err = client.Up(ctx, client2.UpOptions{
			CLIOptions: baseOptions,
			Debug:      cmd.Debug,

			Stdin:  stdinReader,
			Stdout: stdoutWriter,
		})
		if err != nil {
			errChan <- fmt.Errorf("executing up proxy command: %w", err)
		} else {
			errChan <- nil
		}
	}()

	// create container etc.
	result, err := tunnelserver.RunUpServer(
		cancelCtx,
		stdoutReader,
		stdinWriter,
		true,
		true,
		client.WorkspaceConfig(),
		log,
	)
	if err != nil {
		return nil, fmt.Errorf("run tunnel machine: %w", err)
	}

	// wait until command finished
	return result, <-errChan
}

func (cmd *UpCmd) devPodUpDaemon(
	ctx context.Context,
	client client2.DaemonClient,
) (*config2.Result, error) {
	// build devpod up options
	workspace := client.WorkspaceConfig()
	baseOptions := cmd.CLIOptions
	baseOptions.ID = workspace.ID
	baseOptions.DevContainerPath = workspace.DevContainerPath
	baseOptions.DevContainerImage = workspace.DevContainerImage
	baseOptions.IDE = workspace.IDE.Name
	baseOptions.IDEOptions = nil
	baseOptions.Source = workspace.Source.String()
	for optionName, optionValue := range workspace.IDE.Options {
		baseOptions.IDEOptions = append(
			baseOptions.IDEOptions,
			optionName+"="+optionValue.Value,
		)
	}

	// run devpod up elsewhere
	return client.Up(ctx, client2.UpOptions{
		CLIOptions: baseOptions,
		Debug:      cmd.Debug,
	})
}

func (cmd *UpCmd) devPodUpMachine(
	ctx context.Context,
	devPodConfig *config.Config,
	client client2.WorkspaceClient,
	log log.Logger,
) (*config2.Result, error) {
	err := clientimplementation.StartWait(ctx, client, true, log)
	if err != nil {
		return nil, err
	}

	// compress info
	workspaceInfo, wInfo, err := client.AgentInfo(cmd.CLIOptions)
	if err != nil {
		return nil, err
	}

	// create container etc.
	log.Info("creating devcontainer")
	defer log.Debug("done creating devcontainer")

	// if we run on a platform, we need to pass the platform options
	if cmd.Platform.Enabled {
		return clientimplementation.BuildAgentClient(ctx, clientimplementation.BuildAgentClientOptions{
			WorkspaceClient: client,
			CLIOptions:      cmd.CLIOptions,
			AgentCommand:    "up",
			Log:             log,
			TunnelOptions:   []tunnelserver.Option{tunnelserver.WithPlatformOptions(&cmd.Platform)},
		})
	}

	// ssh tunnel command
	sshTunnelCmd := fmt.Sprintf("'%s' helper ssh-server --stdio", client.AgentPath())
	if log.GetLevel() == logrus.DebugLevel {
		sshTunnelCmd += " --debug"
	}

	// create agent command
	agentCommand := fmt.Sprintf(
		"'%s' agent workspace up --workspace-info '%s'",
		client.AgentPath(),
		workspaceInfo,
	)

	if log.GetLevel() == logrus.DebugLevel {
		agentCommand += " --debug"
	}

	agentInjectFunc := func(cancelCtx context.Context, sshCmd string, sshTunnelStdinReader, sshTunnelStdoutWriter *os.File, writer io.WriteCloser) error {
		return agent.InjectAgent(&agent.InjectOptions{
			Ctx: cancelCtx,
			Exec: func(ctx context.Context, command string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
				return client.Command(ctx, client2.CommandOptions{
					Command: command,
					Stdin:   stdin,
					Stdout:  stdout,
					Stderr:  stderr,
				})
			},
			IsLocal:         client.AgentLocal(),
			RemoteAgentPath: client.AgentPath(),
			DownloadURL:     client.AgentURL(),
			Command:         sshCmd,
			Stdin:           sshTunnelStdinReader,
			Stdout:          sshTunnelStdoutWriter,
			Stderr:          writer,
			Log:             log.ErrorStreamOnly(),
			Timeout:         wInfo.InjectTimeout,
		})
	}

	return sshtunnel.ExecuteCommand(sshtunnel.ExecuteCommandOptions{
		Ctx:            ctx,
		Client:         client,
		AddPrivateKeys: devPodConfig.ContextOption(config.ContextOptionSSHAddPrivateKeys) == "true",
		AgentInject:    agentInjectFunc,
		SSHCommand:     sshTunnelCmd,
		Command:        agentCommand,
		Log:            log,
		TunnelServerFunc: func(ctx context.Context, stdin io.WriteCloser, stdout io.Reader) (*config2.Result, error) {
			return tunnelserver.RunUpServer(
				ctx,
				stdout,
				stdin,
				client.AgentInjectGitCredentials(cmd.CLIOptions),
				client.AgentInjectDockerCredentials(cmd.CLIOptions),
				client.WorkspaceConfig(),
				log,
			)
		},
	})
}

func startJupyterNotebookInBrowser(
	forwardGpg bool,
	ctx context.Context,
	devPodConfig *config.Config,
	client client2.BaseWorkspaceClient,
	user string,
	ideOptions map[string]config.OptionValue,
	authSockID string,
	logger log.Logger,
) error {
	if forwardGpg {
		err := performGpgForwarding(client, logger)
		if err != nil {
			return err
		}
	}

	// determine port
	jupyterAddress, jupyterPort, err := parseAddressAndPort(
		jupyter.Options.GetValue(ideOptions, jupyter.BindAddressOption),
		jupyter.DefaultServerPort,
	)
	if err != nil {
		return err
	}

	// wait until reachable then open browser
	targetURL := fmt.Sprintf("http://localhost:%d/lab", jupyterPort)
	if jupyter.Options.GetValue(ideOptions, jupyter.OpenOption) == "true" {
		go func() {
			err = open2.Open(ctx, targetURL, logger)
			if err != nil {
				logger.WithFields(logrus.Fields{"error": err}).Error("error opening jupyter notebook")
			}

			logger.Info("started jupyter notebook in browser mode. Please keep this terminal open as long as you use Jupyter Notebook")
		}()
	}

	// start in browser
	logger.Infof("Starting jupyter notebook in browser mode at %s", targetURL)
	extraPorts := []string{fmt.Sprintf("%s:%d", jupyterAddress, jupyter.DefaultServerPort)}
	return startBrowserTunnel(
		ctx,
		devPodConfig,
		client,
		user,
		targetURL,
		false,
		extraPorts,
		authSockID,
		logger,
	)
}

func startRStudioInBrowser(
	forwardGpg bool,
	ctx context.Context,
	devPodConfig *config.Config,
	client client2.BaseWorkspaceClient,
	user string,
	ideOptions map[string]config.OptionValue,
	authSockID string,
	logger log.Logger,
) error {
	if forwardGpg {
		err := performGpgForwarding(client, logger)
		if err != nil {
			return err
		}
	}

	// determine port
	addr, port, err := parseAddressAndPort(
		rstudio.Options.GetValue(ideOptions, rstudio.BindAddressOption),
		rstudio.DefaultServerPort,
	)
	if err != nil {
		return err
	}

	// wait until reachable then open browser
	targetURL := fmt.Sprintf("http://localhost:%d", port)
	if rstudio.Options.GetValue(ideOptions, rstudio.OpenOption) == "true" {
		go func() {
			err = open2.Open(ctx, targetURL, logger)
			if err != nil {
				logger.Errorf("error opening rstudio: %v", err)
			}

			logger.Infof(
				"started RStudio Server in browser mode. Please keep this terminal open as long as you use it",
			)
		}()
	}

	// start in browser
	logger.Infof("Starting RStudio server in browser mode at %s", targetURL)
	extraPorts := []string{fmt.Sprintf("%s:%d", addr, rstudio.DefaultServerPort)}
	return startBrowserTunnel(
		ctx,
		devPodConfig,
		client,
		user,
		targetURL,
		false,
		extraPorts,
		authSockID,
		logger,
	)
}

func startFleet(ctx context.Context, client client2.BaseWorkspaceClient, logger log.Logger) error {
	// create ssh command
	stdout := &bytes.Buffer{}
	cmd, err := createSSHCommand(
		ctx,
		client,
		logger,
		[]string{"--command", "cat " + fleet.FleetURLFile},
	)
	if err != nil {
		return err
	}
	cmd.Stdout = stdout
	err = cmd.Run()
	if err != nil {
		return command.WrapCommandError(stdout.Bytes(), err)
	}

	url := strings.TrimSpace(stdout.String())
	if len(url) == 0 {
		return fmt.Errorf("seems like fleet is not running within the container")
	}

	logger.Warnf(
		"Fleet is exposed at a publicly reachable URL, please make sure to not disclose this URL to anyone as they will be able to reach your workspace from that",
	)
	logger.Infof("Starting Fleet at %s ...", url)
	err = open.Run(url)
	if err != nil {
		return err
	}

	return nil
}

func startVSCodeInBrowser(
	forwardGpg bool,
	ctx context.Context,
	devPodConfig *config.Config,
	client client2.BaseWorkspaceClient,
	workspaceFolder, user string,
	ideOptions map[string]config.OptionValue,
	authSockID string,
	logger log.Logger,
) error {
	if forwardGpg {
		err := performGpgForwarding(client, logger)
		if err != nil {
			return err
		}
	}

	// determine port
	vscodeAddress, vscodePort, err := parseAddressAndPort(
		openvscode.Options.GetValue(ideOptions, openvscode.BindAddressOption),
		openvscode.DefaultVSCodePort,
	)
	if err != nil {
		return err
	}

	// wait until reachable then open browser
	targetURL := fmt.Sprintf("http://localhost:%d/?folder=%s", vscodePort, workspaceFolder)
	if openvscode.Options.GetValue(ideOptions, openvscode.OpenOption) == "true" {
		go func() {
			err = open2.Open(ctx, targetURL, logger)
			if err != nil {
				logger.Errorf("error opening vscode: %v", err)
			}

			logger.Infof(
				"started vscode in browser mode. Please keep this terminal open as long as you use VSCode browser version",
			)
		}()
	}

	// start in browser
	logger.Infof("Starting vscode in browser mode at %s", targetURL)
	forwardPorts := openvscode.Options.GetValue(ideOptions, openvscode.ForwardPortsOption) == "true"
	extraPorts := []string{fmt.Sprintf("%s:%d", vscodeAddress, openvscode.DefaultVSCodePort)}
	return startBrowserTunnel(
		ctx,
		devPodConfig,
		client,
		user,
		targetURL,
		forwardPorts,
		extraPorts,
		authSockID,
		logger,
	)
}

func parseAddressAndPort(bindAddressOption string, defaultPort int) (string, int, error) {
	var (
		err      error
		address  string
		portName int
	)
	if bindAddressOption == "" {
		portName, err = port.FindAvailablePort(defaultPort)
		if err != nil {
			return "", 0, err
		}

		address = fmt.Sprintf("%d", portName)
	} else {
		address = bindAddressOption
		_, port, err := net.SplitHostPort(address)
		if err != nil {
			return "", 0, fmt.Errorf("parse host:port: %w", err)
		} else if port == "" {
			return "", 0, fmt.Errorf("parse ADDRESS: expected host:port, got %s", address)
		}

		portName, err = strconv.Atoi(port)
		if err != nil {
			return "", 0, fmt.Errorf("parse host:port: %w", err)
		}
	}

	return address, portName, nil
}

// setupBackhaul sets up a long running command in the container to ensure an SSH connection is kept alive
func setupBackhaul(client client2.BaseWorkspaceClient, authSockId string, log log.Logger) error {
	execPath, err := os.Executable()
	if err != nil {
		return err
	}

	remoteUser, err := devssh.GetUser(client.WorkspaceConfig().ID, client.WorkspaceConfig().SSHConfigPath, client.WorkspaceConfig().SSHConfigIncludePath)
	if err != nil {
		remoteUser = "root"
	}

	dotCmd := exec.Command(
		execPath,
		"ssh",
		"--agent-forwarding=true",
		fmt.Sprintf("--reuse-ssh-auth-sock=%s", authSockId),
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

	if log.GetLevel() == logrus.DebugLevel {
		dotCmd.Args = append(dotCmd.Args, "--debug")
	}

	log.Info("Setting up backhaul SSH connection")

	writer := log.Writer(logrus.InfoLevel, false)

	dotCmd.Stdout = writer
	dotCmd.Stderr = writer

	err = dotCmd.Run()
	if err != nil {
		return err
	}

	log.Infof("Done setting up backhaul")

	return nil
}

func startBrowserTunnel(
	ctx context.Context,
	devPodConfig *config.Config,
	client client2.BaseWorkspaceClient,
	user, targetURL string,
	forwardPorts bool,
	extraPorts []string,
	authSockID string,
	logger log.Logger,
) error {
	// Setup a backhaul SSH connection using the remote user so there is an AUTH SOCK to use
	// With normal IDEs this would be the SSH connection made by the IDE
	// authSockID is not set when in proxy mode since we cannot use the proxies ssh-agent
	if authSockID != "" {
		go func() {
			if err := setupBackhaul(client, authSockID, logger); err != nil {
				logger.Error("Failed to setup backhaul SSH connection: ", err)
			}
		}()
	}

	// handle this directly with the daemon client
	daemonClient, ok := client.(client2.DaemonClient)
	if ok {
		toolClient, _, err := daemonClient.SSHClients(ctx, user)
		if err != nil {
			return err
		}
		defer func() { _ = toolClient.Close() }()

		err = clientimplementation.StartServicesDaemon(ctx, clientimplementation.StartServicesDaemonOptions{
			DevPodConfig: devPodConfig,
			Client:       daemonClient,
			SSHClient:    toolClient,
			User:         user,
			Log:          logger,
			ForwardPorts: forwardPorts,
			ExtraPorts:   extraPorts,
		})
		if err != nil {
			return err
		}
		<-ctx.Done()

		return nil
	}

	err := tunnel.NewTunnel(
		ctx,
		func(ctx context.Context, stdin io.Reader, stdout io.Writer) error {
			writer := logger.Writer(logrus.DebugLevel, false)
			defer func() { _ = writer.Close() }()

			cmd, err := createSSHCommand(ctx, client, logger, []string{
				"--log-output=raw",
				fmt.Sprintf("--reuse-ssh-auth-sock=%s", authSockID),
				"--stdio",
			})
			if err != nil {
				return err
			}
			cmd.Stdout = stdout
			cmd.Stdin = stdin
			cmd.Stderr = writer
			return cmd.Run()
		},
		func(ctx context.Context, containerClient *ssh.Client) error {
			// print port to console
			streamLogger, ok := logger.(*log.StreamLogger)
			if ok {
				streamLogger.JSON(logrus.InfoLevel, map[string]string{
					"url":  targetURL,
					"done": "true",
				})
			}

			configureDockerCredentials := devPodConfig.ContextOption(config.ContextOptionSSHInjectDockerCredentials) == "true"
			configureGitCredentials := devPodConfig.ContextOption(config.ContextOptionSSHInjectGitCredentials) == "true"
			configureGitSSHSignatureHelper := devPodConfig.ContextOption(config.ContextOptionGitSSHSignatureForwarding) == "true"

			// run in container
			err := tunnel.RunServices(
				ctx,
				tunnel.RunServicesOptions{
					DevPodConfig:                   devPodConfig,
					ContainerClient:                containerClient,
					User:                           user,
					ForwardPorts:                   forwardPorts,
					ExtraPorts:                     extraPorts,
					PlatformOptions:                nil,
					Workspace:                      client.WorkspaceConfig(),
					ConfigureDockerCredentials:     configureDockerCredentials,
					ConfigureGitCredentials:        configureGitCredentials,
					ConfigureGitSSHSignatureHelper: configureGitSSHSignatureHelper,
					Log:                            logger,
				},
			)
			if err != nil {
				return fmt.Errorf("run credentials server in browser tunnel: %w", err)
			}

			<-ctx.Done()
			return nil
		},
	)
	if err != nil {
		return err
	}

	return nil
}
