package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"

	"github.com/blang/semver/v4"
	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/cmd/flags"
	"github.com/skevetter/devpod/pkg/agent"
	"github.com/skevetter/devpod/pkg/agent/tunnelserver"
	client2 "github.com/skevetter/devpod/pkg/client"
	"github.com/skevetter/devpod/pkg/client/clientimplementation"
	"github.com/skevetter/devpod/pkg/command"
	"github.com/skevetter/devpod/pkg/config"
	config2 "github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/skevetter/devpod/pkg/devcontainer/sshtunnel"
	"github.com/skevetter/devpod/pkg/ide"
	"github.com/skevetter/devpod/pkg/ide/fleet"
	"github.com/skevetter/devpod/pkg/ide/jetbrains"
	"github.com/skevetter/devpod/pkg/ide/jupyter"
	"github.com/skevetter/devpod/pkg/ide/openvscode"
	"github.com/skevetter/devpod/pkg/ide/rstudio"
	"github.com/skevetter/devpod/pkg/ide/vscode"
	"github.com/skevetter/devpod/pkg/ide/zed"
	open2 "github.com/skevetter/devpod/pkg/open"
	options2 "github.com/skevetter/devpod/pkg/options"
	"github.com/skevetter/devpod/pkg/platform"
	"github.com/skevetter/devpod/pkg/port"
	provider2 "github.com/skevetter/devpod/pkg/provider"
	devssh "github.com/skevetter/devpod/pkg/ssh"
	"github.com/skevetter/devpod/pkg/telemetry"
	"github.com/skevetter/devpod/pkg/tunnel"
	"github.com/skevetter/devpod/pkg/util"
	"github.com/skevetter/devpod/pkg/version"
	workspace2 "github.com/skevetter/devpod/pkg/workspace"
	"github.com/skevetter/log"
	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"
)

// UpCmd holds the up cmd flags
type UpCmd struct {
	provider2.CLIOptions
	*flags.GlobalFlags

	Machine string

	ProviderOptions []string

	ConfigureSSH       bool
	GPGAgentForwarding bool
	OpenIDE            bool
	Reconfigure        bool

	SSHConfigPath string

	DotfilesSource        string
	DotfilesScript        string
	DotfilesScriptEnv     []string // Key=Value to pass to install script
	DotfilesScriptEnvFile []string // Paths to files containing Key=Value pairs to pass to install script
}

// NewUpCmd creates a new up command
func NewUpCmd(f *flags.GlobalFlags) *cobra.Command {
	cmd := &UpCmd{GlobalFlags: f}
	upCmd := &cobra.Command{
		Use:   "up [flags] [workspace-path|workspace-name]",
		Short: "Starts a new workspace",
		RunE:  cmd.execute,
	}
	cmd.registerFlags(upCmd)
	return upCmd
}

func (cmd *UpCmd) execute(cobraCmd *cobra.Command, args []string) error {
	if err := cmd.validate(); err != nil {
		return err
	}
	devPodConfig, err := config.LoadConfig(cmd.Context, cmd.Provider)
	if err != nil {
		return err
	}
	if devPodConfig.ContextOption(config.ContextOptionSSHStrictHostKeyChecking) == "true" {
		cmd.StrictHostKeyChecking = true
	}

	ctx, cancel := WithSignals(cobraCmd.Context())
	defer cancel()

	client, logger, err := cmd.prepareClient(ctx, devPodConfig, args)
	if err != nil {
		return fmt.Errorf("prepare workspace client: %w", err)
	}
	if cmd.ExtraDevContainerPath != "" && client.Provider() != "docker" {
		return fmt.Errorf("extra devcontainer file is only supported with local provider")
	}

	telemetry.CollectorCLI.SetClient(client)
	return cmd.Run(ctx, devPodConfig, client, args, logger)
}

func (cmd *UpCmd) validate() error {
	if err := validatePodmanFlags(cmd); err != nil {
		return err
	}
	if cmd.ExtraDevContainerPath != "" {
		absPath, err := filepath.Abs(cmd.ExtraDevContainerPath)
		if err != nil {
			return err
		}
		cmd.ExtraDevContainerPath = absPath
	}
	return nil
}

func (cmd *UpCmd) registerFlags(upCmd *cobra.Command) {
	cmd.registerSSHFlags(upCmd)
	cmd.registerDotfilesFlags(upCmd)
	cmd.registerDevContainerFlags(upCmd)
	cmd.registerIDEFlags(upCmd)
	cmd.registerGitFlags(upCmd)
	cmd.registerPodmanFlags(upCmd)
	cmd.registerWorkspaceFlags(upCmd)
	cmd.registerTestingFlags(upCmd)
}

func (cmd *UpCmd) registerSSHFlags(upCmd *cobra.Command) {
	upCmd.Flags().BoolVar(&cmd.ConfigureSSH, "configure-ssh", true, "If true will configure the ssh config to include the DevPod workspace")
	upCmd.Flags().BoolVar(&cmd.GPGAgentForwarding, "gpg-agent-forwarding", false, "If true forward the local gpg-agent to the DevPod workspace")
	upCmd.Flags().StringVar(&cmd.SSHConfigPath, "ssh-config", "", "The path to the ssh config to modify, if empty will use ~/.ssh/config")
}

func (cmd *UpCmd) registerDotfilesFlags(upCmd *cobra.Command) {
	upCmd.Flags().StringVar(&cmd.DotfilesSource, "dotfiles", "", "The path or url to the dotfiles to use in the container")
	upCmd.Flags().StringVar(&cmd.DotfilesScript, "dotfiles-script", "", "The path in dotfiles directory to use to install the dotfiles, if empty will try to guess")
	upCmd.Flags().StringSliceVar(&cmd.DotfilesScriptEnv, "dotfiles-script-env", []string{}, "Extra environment variables to put into the dotfiles install script. E.g. MY_ENV_VAR=MY_VALUE")
	upCmd.Flags().StringSliceVar(&cmd.DotfilesScriptEnvFile, "dotfiles-script-env-file", []string{}, "The path to files containing environment variables to set for the dotfiles install script")
}

func (cmd *UpCmd) registerDevContainerFlags(upCmd *cobra.Command) {
	upCmd.Flags().StringVar(&cmd.DevContainerImage, "devcontainer-image", "", "The container image to use, this will override the devcontainer.json value in the project")
	upCmd.Flags().StringVar(&cmd.DevContainerPath, "devcontainer-path", "", "The path to the devcontainer.json relative to the project")
	upCmd.Flags().StringVar(&cmd.DevContainerID, "devcontainer-id", "", "The ID of the devcontainer to use when multiple exist (e.g., folder name in .devcontainer/FOLDER/devcontainer.json)")
	upCmd.Flags().StringVar(&cmd.ExtraDevContainerPath, "extra-devcontainer-path", "", "The path to an additional devcontainer.json file to override original devcontainer.json")
	upCmd.Flags().StringVar(&cmd.FallbackImage, "fallback-image", "", "The fallback image to use if no devcontainer configuration has been detected")
}

func (cmd *UpCmd) registerIDEFlags(upCmd *cobra.Command) {
	upCmd.Flags().StringVar(&cmd.IDE, "ide", "", "The IDE to open the workspace in. If empty will use vscode locally or in browser")
	upCmd.Flags().StringArrayVar(&cmd.IDEOptions, "ide-option", []string{}, "IDE option in the form KEY=VALUE")
	upCmd.Flags().BoolVar(&cmd.OpenIDE, "open-ide", true, "If this is false and an IDE is configured, DevPod will only install the IDE server backend, but not open it")
}

func (cmd *UpCmd) registerGitFlags(upCmd *cobra.Command) {
	upCmd.Flags().Var(&cmd.GitCloneStrategy, "git-clone-strategy", "The git clone strategy DevPod uses to checkout git based workspaces. Can be full (default), blobless, treeless or shallow")
	upCmd.Flags().BoolVar(&cmd.GitCloneRecursiveSubmodules, "git-clone-recursive-submodules", false, "If true will clone git submodule repositories recursively")
	upCmd.Flags().StringVar(&cmd.GitSSHSigningKey, "git-ssh-signing-key", "", "The ssh key to use when signing git commits. Used to explicitly setup DevPod's ssh signature forwarding with given key. Should be same format as value of `git config user.signingkey`")
}

func (cmd *UpCmd) registerPodmanFlags(upCmd *cobra.Command) {
	upCmd.Flags().StringVar(&cmd.Userns, "userns", "", "User namespace to use for the container (Podman only; e.g. \"keep-id\", \"host\", or \"auto\")")
	upCmd.Flags().StringSliceVar(&cmd.UidMap, "uidmap", []string{}, "UID mapping for Podman user namespace (Podman only; format: container_id:host_id:amount, e.g. \"0:1000:1\")")
	upCmd.Flags().StringSliceVar(&cmd.GidMap, "gidmap", []string{}, "GID mapping for Podman user namespace (Podman only; format: container_id:host_id:amount, e.g. \"0:1000:1\")")
}

func (cmd *UpCmd) registerWorkspaceFlags(upCmd *cobra.Command) {
	upCmd.Flags().StringVar(&cmd.ID, "id", "", "The id to use for the workspace")
	upCmd.Flags().StringVar(&cmd.Machine, "machine", "", "The machine to use for this workspace. The machine needs to exist beforehand or the command will fail. If the workspace already exists, this option has no effect")
	upCmd.Flags().StringVar(&cmd.Source, "source", "", "Optional source for the workspace. E.g. git:https://github.com/my-org/my-repo")
	upCmd.Flags().StringArrayVar(&cmd.ProviderOptions, "provider-option", []string{}, "Provider option in the form KEY=VALUE")
	upCmd.Flags().BoolVar(&cmd.Reconfigure, "reconfigure", false, "Reconfigure the options for this workspace. Only supported in DevPod Pro right now.")
	upCmd.Flags().BoolVar(&cmd.Recreate, "recreate", false, "If true will remove any existing containers and recreate them")
	upCmd.Flags().BoolVar(&cmd.Reset, "reset", false, "If true will remove any existing containers including sources, and recreate them")
	upCmd.Flags().StringSliceVar(&cmd.PrebuildRepositories, "prebuild-repository", []string{}, "Docker repository that hosts devpod prebuilds for this workspace")
	upCmd.Flags().StringArrayVar(&cmd.WorkspaceEnv, "workspace-env", []string{}, "Extra env variables to put into the workspace. E.g. MY_ENV_VAR=MY_VALUE")
	upCmd.Flags().StringSliceVar(&cmd.WorkspaceEnvFile, "workspace-env-file", []string{}, "The path to files containing a list of extra env variables to put into the workspace. E.g. MY_ENV_VAR=MY_VALUE")
	upCmd.Flags().StringArrayVar(&cmd.InitEnv, "init-env", []string{}, "Extra env variables to inject during the initialization of the workspace. E.g. MY_ENV_VAR=MY_VALUE")
	upCmd.Flags().BoolVar(&cmd.DisableDaemon, "disable-daemon", false, "If enabled, will not install a daemon into the target machine to track activity")
}

func (cmd *UpCmd) registerTestingFlags(upCmd *cobra.Command) {
	upCmd.Flags().StringVar(&cmd.DaemonInterval, "daemon-interval", "", "TESTING ONLY")
	_ = upCmd.Flags().MarkHidden("daemon-interval")
	upCmd.Flags().BoolVar(&cmd.ForceDockerless, "force-dockerless", false, "TESTING ONLY")
	_ = upCmd.Flags().MarkHidden("force-dockerless")
}

// Run runs the command logic
func (cmd *UpCmd) Run(
	ctx context.Context,
	devPodConfig *config.Config,
	client client2.BaseWorkspaceClient,
	args []string,
	log log.Logger,
) error {
	cmd.prepareWorkspace(client, log)

	wctx, err := cmd.executeDevPodUp(ctx, devPodConfig, client, log)
	if err != nil {
		return err
	}
	if wctx == nil {
		return nil // Platform mode
	}

	if err := cmd.configureWorkspace(devPodConfig, client, wctx, log); err != nil {
		return err
	}

	return cmd.openIDE(ctx, devPodConfig, client, wctx, log)
}

// workspaceContext holds the result of workspace preparation
type workspaceContext struct {
	result  *config2.Result
	user    string
	workdir string
}

// prepareWorkspace handles initial setup and validation
func (cmd *UpCmd) prepareWorkspace(client client2.BaseWorkspaceClient, log log.Logger) {
	if cmd.Reset {
		cmd.Recreate = true
	}

	targetIDE := client.WorkspaceConfig().IDE.Name
	if cmd.IDE != "" {
		targetIDE = cmd.IDE
	}

	if !cmd.Platform.Enabled && ide.ReusesAuthSock(targetIDE) {
		cmd.SSHAuthSockID = util.RandStringBytes(10)
		log.Debug("Reusing SSH_AUTH_SOCK", cmd.SSHAuthSockID)
	} else if cmd.Platform.Enabled && ide.ReusesAuthSock(targetIDE) {
		log.Debug("Reusing SSH_AUTH_SOCK is not supported with platform mode, consider launching the IDE from the platform UI")
	}
}

// executeDevPodUp runs the agent and returns workspace context
func (cmd *UpCmd) executeDevPodUp(ctx context.Context, devPodConfig *config.Config, client client2.BaseWorkspaceClient, log log.Logger) (*workspaceContext, error) {
	result, err := cmd.devPodUp(ctx, devPodConfig, client, log)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("did not receive a result back from agent")
	}
	if cmd.Platform.Enabled {
		return nil, nil
	}

	user := config2.GetRemoteUser(result)
	workdir := ""
	if result.MergedConfig != nil && result.MergedConfig.WorkspaceFolder != "" {
		workdir = result.MergedConfig.WorkspaceFolder
	}
	if client.WorkspaceConfig().Source.GitSubPath != "" {
		result.SubstitutionContext.ContainerWorkspaceFolder = filepath.Join(result.SubstitutionContext.ContainerWorkspaceFolder, client.WorkspaceConfig().Source.GitSubPath)
		workdir = result.SubstitutionContext.ContainerWorkspaceFolder
	}

	return &workspaceContext{result: result, user: user, workdir: workdir}, nil
}

// configureWorkspace sets up SSH, Git, and dotfiles
func (cmd *UpCmd) configureWorkspace(devPodConfig *config.Config, client client2.BaseWorkspaceClient, wctx *workspaceContext, log log.Logger) error {
	if cmd.ConfigureSSH {
		devPodHome := ""
		if envDevPodHome, ok := os.LookupEnv("DEVPOD_HOME"); ok {
			devPodHome = envDevPodHome
		}
		setupGPGAgentForwarding := cmd.GPGAgentForwarding || devPodConfig.ContextOption(config.ContextOptionGPGAgentForwarding) == "true"
		sshConfigIncludePath := devPodConfig.ContextOption(config.ContextOptionSSHConfigIncludePath)

		if err := configureSSH(client, configureSSHParams{
			sshConfigPath:        cmd.SSHConfigPath,
			sshConfigIncludePath: sshConfigIncludePath,
			user:                 wctx.user,
			workdir:              wctx.workdir,
			gpgagent:             setupGPGAgentForwarding,
			devPodHome:           devPodHome,
		}); err != nil {
			return err
		}

		log.Info("SSH configuration completed in workspace")
	}

	if cmd.GitSSHSigningKey != "" {
		if err := setupGitSSHSignature(cmd.GitSSHSigningKey, client, log); err != nil {
			return err
		}
	}

	return setupDotfiles(cmd.DotfilesSource, cmd.DotfilesScript, cmd.DotfilesScriptEnvFile, cmd.DotfilesScriptEnv, client, devPodConfig, log)
}

// openIDE opens the configured IDE
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

	return sshtunnel.ExecuteCommand(
		ctx,
		client,
		devPodConfig.ContextOption(config.ContextOptionSSHAddPrivateKeys) == "true",
		agentInjectFunc,
		sshTunnelCmd,
		agentCommand,
		log,
		func(ctx context.Context, stdin io.WriteCloser, stdout io.Reader) (*config2.Result, error) {
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
	)
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

type configureSSHParams struct {
	sshConfigPath        string
	sshConfigIncludePath string
	user                 string
	workdir              string
	gpgagent             bool
	devPodHome           string
}

func configureSSH(client client2.BaseWorkspaceClient, params configureSSHParams) error {
	path, err := devssh.ResolveSSHConfigPath(params.sshConfigPath)
	if err != nil {
		return fmt.Errorf("invalid ssh config path: %w", err)
	}
	sshConfigPath := path

	sshConfigIncludePath := params.sshConfigIncludePath
	if sshConfigIncludePath != "" {
		includePath, err := devssh.ResolveSSHConfigPath(sshConfigIncludePath)
		if err != nil {
			return fmt.Errorf("invalid ssh config include path: %w", err)
		}
		sshConfigIncludePath = includePath
	}

	err = devssh.ConfigureSSHConfig(devssh.SSHConfigParams{
		SSHConfigPath:        sshConfigPath,
		SSHConfigIncludePath: sshConfigIncludePath,
		Context:              client.Context(),
		Workspace:            client.Workspace(),
		User:                 params.user,
		Workdir:              params.workdir,
		GPGAgent:             params.gpgagent,
		DevPodHome:           params.devPodHome,
		Provider:             client.Provider(),
		Log:                  log.Default,
	})
	if err != nil {
		return err
	}

	return nil
}

func mergeDevPodUpOptions(baseOptions *provider2.CLIOptions) error {
	oldOptions := *baseOptions
	found, err := clientimplementation.DecodeOptionsFromEnv(
		clientimplementation.DevPodFlagsUp,
		baseOptions,
	)
	if err != nil {
		return fmt.Errorf("decode up options: %w", err)
	} else if found {
		baseOptions.WorkspaceEnv = append(oldOptions.WorkspaceEnv, baseOptions.WorkspaceEnv...)
		baseOptions.InitEnv = append(oldOptions.InitEnv, baseOptions.InitEnv...)
		baseOptions.PrebuildRepositories = append(oldOptions.PrebuildRepositories, baseOptions.PrebuildRepositories...)
		baseOptions.IDEOptions = append(oldOptions.IDEOptions, baseOptions.IDEOptions...)
	}

	err = clientimplementation.DecodePlatformOptionsFromEnv(&baseOptions.Platform)
	if err != nil {
		return fmt.Errorf("decode platform options: %w", err)
	}

	return nil
}

func mergeEnvFromFiles(baseOptions *provider2.CLIOptions) error {
	var variables []string
	for _, file := range baseOptions.WorkspaceEnvFile {
		envFromFile, err := config2.ParseKeyValueFile(file)
		if err != nil {
			return err
		}
		variables = append(variables, envFromFile...)
	}
	baseOptions.WorkspaceEnv = append(baseOptions.WorkspaceEnv, variables...)

	return nil
}

var inheritedEnvironmentVariables = []string{
	"GIT_AUTHOR_NAME",
	"GIT_AUTHOR_EMAIL",
	"GIT_AUTHOR_DATE",
	"GIT_COMMITTER_NAME",
	"GIT_COMMITTER_EMAIL",
	"GIT_COMMITTER_DATE",
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

	return exec.CommandContext(ctx, execPath, args...), nil
}

func setupDotfiles(
	dotfiles, script string,
	envFiles, envKeyValuePairs []string,
	client client2.BaseWorkspaceClient,
	devPodConfig *config.Config,
	log log.Logger,
) error {
	dotfilesRepo := devPodConfig.ContextOption(config.ContextOptionDotfilesURL)
	if dotfiles != "" {
		dotfilesRepo = dotfiles
	}

	dotfilesScript := devPodConfig.ContextOption(config.ContextOptionDotfilesScript)
	if script != "" {
		dotfilesScript = script
	}

	if dotfilesRepo == "" {
		log.Debug("No dotfiles repo specified, skipping")
		return nil
	}

	log.Infof("Dotfiles git repository %s specified", dotfilesRepo)
	log.Debug("Cloning dotfiles into the devcontainer...")

	dotCmd, err := buildDotCmd(devPodConfig, dotfilesRepo, dotfilesScript, envFiles, envKeyValuePairs, client, log)
	if err != nil {
		return err
	}
	if log.GetLevel() == logrus.DebugLevel {
		dotCmd.Args = append(dotCmd.Args, "--debug")
	}

	log.Debugf("Running dotfiles setup command: %v", dotCmd.Args)

	writer := log.Writer(logrus.InfoLevel, false)

	dotCmd.Stdout = writer
	dotCmd.Stderr = writer

	err = dotCmd.Run()
	if err != nil {
		return err
	}

	log.Infof("Done setting up dotfiles into the devcontainer")

	return nil
}

func buildDotCmdAgentArguments(devPodConfig *config.Config, dotfilesRepo, dotfilesScript string, log log.Logger) []string {
	agentArguments := []string{
		"agent",
		"workspace",
		"install-dotfiles",
		"--repository",
		dotfilesRepo,
	}

	if devPodConfig.ContextOption(config.ContextOptionSSHStrictHostKeyChecking) == "true" {
		agentArguments = append(agentArguments, "--strict-host-key-checking")
	}

	if log.GetLevel() == logrus.DebugLevel {
		agentArguments = append(agentArguments, "--debug")
	}

	if dotfilesScript != "" {
		log.Infof("Dotfiles script %s specified", dotfilesScript)
		agentArguments = append(agentArguments, "--install-script", dotfilesScript)
	}

	return agentArguments
}

func buildDotCmd(devPodConfig *config.Config, dotfilesRepo, dotfilesScript string, envFiles, envKeyValuePairs []string, client client2.BaseWorkspaceClient, log log.Logger) (*exec.Cmd, error) {
	sshCmd := []string{
		"ssh",
		"--agent-forwarding=true",
		"--start-services=true",
	}

	envFilesKeyValuePairs, err := collectDotfilesScriptEnvKeyvaluePairs(envFiles)
	if err != nil {
		return nil, err
	}

	// Collect file-based and CLI options env variables names (aka keys) and
	// configure ssh env var passthrough with send-env
	allEnvKeyValuesPairs := slices.Concat(envFilesKeyValuePairs, envKeyValuePairs)
	allEnvKeys := extractKeysFromEnvKeyValuePairs(allEnvKeyValuesPairs)
	for _, envKey := range allEnvKeys {
		sshCmd = append(sshCmd, "--send-env", envKey)
	}

	remoteUser, err := devssh.GetUser(client.WorkspaceConfig().ID, client.WorkspaceConfig().SSHConfigPath, client.WorkspaceConfig().SSHConfigIncludePath)
	if err != nil {
		remoteUser = "root"
	}

	agentArguments := buildDotCmdAgentArguments(devPodConfig, dotfilesRepo, dotfilesScript, log)
	sshCmd = append(sshCmd,
		"--user",
		remoteUser,
		"--context",
		client.Context(),
		client.Workspace(),
		"--log-output=raw",
		"--command",
		agent.ContainerDevPodHelperLocation+" "+strings.Join(agentArguments, " "),
	)
	execPath, err := os.Executable()
	if err != nil {
		return nil, err
	}

	dotCmd := exec.Command(
		execPath,
		sshCmd...,
	)

	dotCmd.Env = append(dotCmd.Environ(), allEnvKeyValuesPairs...)
	return dotCmd, nil
}

func extractKeysFromEnvKeyValuePairs(envKeyValuePairs []string) []string {
	keys := []string{}
	for _, env := range envKeyValuePairs {
		keyValue := strings.SplitN(env, "=", 2)
		if len(keyValue) == 2 {
			keys = append(keys, keyValue[0])
		}
	}
	return keys
}

func collectDotfilesScriptEnvKeyvaluePairs(envFiles []string) ([]string, error) {
	keyValues := []string{}
	for _, file := range envFiles {
		envFromFile, err := config2.ParseKeyValueFile(file)
		if err != nil {
			return nil, err
		}
		keyValues = append(keyValues, envFromFile...)
	}
	return keyValues, nil
}

func setupGitSSHSignature(signingKey string, client client2.BaseWorkspaceClient, log log.Logger) error {
	execPath, err := os.Executable()
	if err != nil {
		return err
	}

	remoteUser, err := devssh.GetUser(client.WorkspaceConfig().ID, client.WorkspaceConfig().SSHConfigPath, client.WorkspaceConfig().SSHConfigIncludePath)
	if err != nil {
		remoteUser = "root"
	}

	err = exec.Command(
		execPath,
		"ssh",
		"--agent-forwarding=true",
		"--start-services=true",
		"--user",
		remoteUser,
		"--context",
		client.Context(),
		client.Workspace(),
		"--command", fmt.Sprintf("devpod agent git-ssh-signature-helper %s", signingKey),
	).Run()
	if err != nil {
		log.Error("failure in setting up git ssh signature helper")
	}
	return nil
}

func performGpgForwarding(
	client client2.BaseWorkspaceClient,
	log log.Logger,
) error {
	log.Debug("gpg forwarding enabled, performing immediately")

	execPath, err := os.Executable()
	if err != nil {
		return err
	}

	remoteUser, err := devssh.GetUser(client.WorkspaceConfig().ID, client.WorkspaceConfig().SSHConfigPath, client.WorkspaceConfig().SSHConfigIncludePath)
	if err != nil {
		remoteUser = "root"
	}

	log.Info("forwarding gpg-agent")

	// perform in background an ssh command forwarding the
	// gpg agent, in order to have it immediately take effect
	go func() {
		err = exec.Command(
			execPath,
			"ssh",
			"--gpg-agent-forwarding=true",
			"--agent-forwarding=true",
			"--start-services=true",
			"--user",
			remoteUser,
			"--context",
			client.Context(),
			client.Workspace(),
			"--log-output=raw",
			"--command", "sleep infinity",
		).Run()
		if err != nil {
			log.Error("failure in forwarding gpg-agent")
		}
	}()

	return nil
}

// checkProviderUpdate currently only ensures the local provider is in sync with the remote for DevPod Pro instances
// Potentially auto-upgrade other providers in the future.
func checkProviderUpdate(devPodConfig *config.Config, proInstance *provider2.ProInstance, log log.Logger) error {
	if version.GetVersion() == version.DevVersion {
		log.Debugf("skipping provider upgrade check during development")
		return nil
	}
	if proInstance == nil {
		log.Debug("no pro instance available, skipping provider upgrade check")
		return nil
	}

	// compare versions
	newVersion, err := platform.GetProInstanceDevPodVersion(proInstance)
	if err != nil {
		return fmt.Errorf("version for pro instance %s: %w", proInstance.Host, err)
	}

	p, err := workspace2.FindProvider(devPodConfig, proInstance.Provider, log)
	if err != nil {
		return fmt.Errorf("get provider config for pro provider %s: %w", proInstance.Provider, err)
	}
	if p.Config.Version == version.DevVersion {
		return nil
	}
	if p.Config.Source.Internal {
		return nil
	}

	v1, err := semver.Parse(strings.TrimPrefix(newVersion, "v"))
	if err != nil {
		return fmt.Errorf("parse version %s: %w", newVersion, err)
	}
	v2, err := semver.Parse(strings.TrimPrefix(p.Config.Version, "v"))
	if err != nil {
		return fmt.Errorf("parse version %s: %w", p.Config.Version, err)
	}
	if v1.Compare(v2) == 0 {
		return nil
	}
	log.Infof("New provider version available, attempting to update %s from %s to %s", proInstance.Provider, p.Config.Version, newVersion)

	providerSource, err := workspace2.ResolveProviderSource(devPodConfig, proInstance.Provider, log)
	if err != nil {
		return fmt.Errorf("resolve provider source %s: %w", proInstance.Provider, err)
	}

	splitted := strings.Split(providerSource, "@")
	if len(splitted) == 0 {
		return fmt.Errorf("no provider source found %s", providerSource)
	}
	providerSource = splitted[0] + "@" + newVersion

	_, err = workspace2.UpdateProvider(devPodConfig, proInstance.Provider, providerSource, log)
	if err != nil {
		return fmt.Errorf("update provider %s: %w", proInstance.Provider, err)
	}

	log.WithFields(logrus.Fields{
		"provider": proInstance.Provider,
	}).Done("updated provider")
	return nil
}

func getProInstance(devPodConfig *config.Config, providerName string, log log.Logger) *provider2.ProInstance {
	proInstances, err := workspace2.ListProInstances(devPodConfig, log)
	if err != nil {
		return nil
	} else if len(proInstances) == 0 {
		return nil
	}

	proInstance, ok := workspace2.FindProviderProInstance(proInstances, providerName)
	if !ok {
		return nil
	}

	return proInstance
}

func (cmd *UpCmd) prepareClient(ctx context.Context, devPodConfig *config.Config, args []string) (client2.BaseWorkspaceClient, log.Logger, error) {
	// try to parse flags from env
	if err := mergeDevPodUpOptions(&cmd.CLIOptions); err != nil {
		return nil, nil, err
	}

	var logger log.Logger = log.Default
	if cmd.Platform.Enabled {
		logger = logger.ErrorStreamOnly()
		logger.Debug("Running in platform mode")
		logger.Debug("Using error output stream")

		// merge context options from env
		config.MergeContextOptions(devPodConfig.Current(), os.Environ())
	}

	if err := mergeEnvFromFiles(&cmd.CLIOptions); err != nil {
		return nil, logger, err
	}

	cmd.WorkspaceEnv = options2.InheritFromEnvironment(cmd.WorkspaceEnv, inheritedEnvironmentVariables, "")

	var source *provider2.WorkspaceSource
	if cmd.Source != "" {
		source = provider2.ParseWorkspaceSource(cmd.Source)
		if source == nil {
			return nil, nil, fmt.Errorf("workspace source is missing")
		} else if source.LocalFolder != "" && cmd.Platform.Enabled {
			return nil, nil, fmt.Errorf("local folder is not supported in platform mode. Please specify a git repository instead")
		}
	}

	if cmd.SSHConfigPath == "" {
		cmd.SSHConfigPath = devPodConfig.ContextOption(config.ContextOptionSSHConfigPath)
	}
	sshConfigIncludePath := devPodConfig.ContextOption(config.ContextOptionSSHConfigIncludePath)

	client, err := workspace2.Resolve(
		ctx,
		devPodConfig,
		workspace2.ResolveParams{
			IDE:                  cmd.IDE,
			IDEOptions:           cmd.IDEOptions,
			Args:                 args,
			DesiredID:            cmd.ID,
			DesiredMachine:       cmd.Machine,
			ProviderUserOptions:  cmd.ProviderOptions,
			ReconfigureProvider:  cmd.Reconfigure,
			DevContainerImage:    cmd.DevContainerImage,
			DevContainerPath:     cmd.DevContainerPath,
			SSHConfigPath:        cmd.SSHConfigPath,
			SSHConfigIncludePath: sshConfigIncludePath,
			Source:               source,
			UID:                  cmd.UID,
			ChangeLastUsed:       true,
			Owner:                cmd.Owner,
		},
		logger,
	)
	if err != nil {
		return nil, logger, err
	}

	if !cmd.Platform.Enabled {
		proInstance := getProInstance(devPodConfig, client.Provider(), logger)
		err = checkProviderUpdate(devPodConfig, proInstance, logger)
		if err != nil {
			return nil, logger, err
		}
	}

	return client, logger, nil
}

func WithSignals(ctx context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancel(ctx)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		select {
		case <-signals:
			cancel()
		case <-ctx.Done():
		}
	}()

	go func() {
		<-ctx.Done()
		<-signals
		// force shutdown if context is done and we receive another signal
		os.Exit(1)
	}()

	return ctx, func() {
		cancel()
		signal.Stop(signals)
	}
}

func validatePodmanFlags(cmd *UpCmd) error {
	if cmd.Userns != "" && (len(cmd.UidMap) > 0 || len(cmd.GidMap) > 0) {
		return fmt.Errorf("--userns cannot be combined with --uidmap or --gidmap (mutually exclusive)")
	}
	for _, m := range cmd.UidMap {
		if !isValidMapping(m) {
			return fmt.Errorf("invalid --uidmap format: %s (expected: container_id:host_id:amount)", m)
		}
	}
	for _, m := range cmd.GidMap {
		if !isValidMapping(m) {
			return fmt.Errorf("invalid --gidmap format: %s (expected: container_id:host_id:amount)", m)
		}
	}
	return nil
}

func isValidMapping(mapping string) bool {
	parts := strings.Split(mapping, ":")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if _, err := strconv.Atoi(part); err != nil {
			return false
		}
	}
	return true
}
