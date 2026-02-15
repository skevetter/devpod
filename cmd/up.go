package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/blang/semver/v4"
	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/cmd/flags"
	client2 "github.com/skevetter/devpod/pkg/client"
	"github.com/skevetter/devpod/pkg/config"
	config2 "github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/skevetter/devpod/pkg/ide"
	options2 "github.com/skevetter/devpod/pkg/options"
	"github.com/skevetter/devpod/pkg/platform"
	provider2 "github.com/skevetter/devpod/pkg/provider"
	"github.com/skevetter/devpod/pkg/telemetry"
	"github.com/skevetter/devpod/pkg/util"
	"github.com/skevetter/devpod/pkg/version"
	workspace2 "github.com/skevetter/devpod/pkg/workspace"
	"github.com/skevetter/log"
	"github.com/spf13/cobra"
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
