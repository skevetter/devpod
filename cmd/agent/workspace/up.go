package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"errors"

	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/cmd/flags"
	"github.com/skevetter/devpod/pkg/agent"
	"github.com/skevetter/devpod/pkg/agent/tunnel"
	"github.com/skevetter/devpod/pkg/agent/tunnelserver"
	"github.com/skevetter/devpod/pkg/binaries"
	"github.com/skevetter/devpod/pkg/client/clientimplementation"
	"github.com/skevetter/devpod/pkg/command"
	"github.com/skevetter/devpod/pkg/credentials"
	agentdaemon "github.com/skevetter/devpod/pkg/daemon/agent"
	"github.com/skevetter/devpod/pkg/devcontainer"
	config2 "github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/skevetter/devpod/pkg/devcontainer/crane"
	"github.com/skevetter/devpod/pkg/dockercredentials"
	"github.com/skevetter/devpod/pkg/dockerinstall"
	"github.com/skevetter/devpod/pkg/extract"
	provider2 "github.com/skevetter/devpod/pkg/provider"
	"github.com/skevetter/devpod/pkg/util"
	"github.com/skevetter/log"
	"github.com/spf13/cobra"
)

// UpCmd holds the up cmd flags
type UpCmd struct {
	*flags.GlobalFlags

	WorkspaceInfo string
}

// NewUpCmd creates a new command
func NewUpCmd(flags *flags.GlobalFlags) *cobra.Command {
	cmd := &UpCmd{
		GlobalFlags: flags,
	}
	upCmd := &cobra.Command{
		Use:   "up",
		Short: "Starts a new devcontainer",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return cmd.Run(context.Background())
		},
	}
	upCmd.Flags().StringVar(&cmd.WorkspaceInfo, "workspace-info", "", "The workspace info")
	_ = upCmd.MarkFlagRequired("workspace-info")
	return upCmd
}

func (cmd *UpCmd) Run(ctx context.Context) error {
	workspaceInfo, err := cmd.loadWorkspaceInfo(ctx)
	if err != nil {
		return err
	}
	if workspaceInfo == nil {
		return nil
	}

	if cmd.shouldPreventDaemonShutdown(workspaceInfo) {
		agent.CreateWorkspaceBusyFile(workspaceInfo.Origin)
		defer agent.DeleteWorkspaceBusyFile(workspaceInfo.Origin)
	}

	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	tunnelClient, logger, credentialsDir, err := initWorkspace(
		cancelCtx,
		cancel,
		workspaceInfo,
		cmd.Debug,
		cmd.shouldInstallDaemon(workspaceInfo),
	)
	defer cmd.cleanupCredentials(credentialsDir)
	if err != nil {
		return cmd.handleInitError(err, workspaceInfo, logger)
	}

	if err := cmd.up(ctx, workspaceInfo, tunnelClient, logger); err != nil {
		return fmt.Errorf("devcontainer up: %w", err)
	}

	return nil
}

func (cmd *UpCmd) loadWorkspaceInfo(ctx context.Context) (*provider2.AgentWorkspaceInfo, error) {
	shouldExit, workspaceInfo, err := agent.WriteWorkspaceInfoAndDeleteOld(
		cmd.WorkspaceInfo,
		func(workspaceInfo *provider2.AgentWorkspaceInfo, log log.Logger) error {
			return deleteWorkspace(ctx, workspaceInfo, log)
		},
		log.Default.ErrorStreamOnly(),
	)
	if err != nil {
		return nil, fmt.Errorf("error parsing workspace info: %w", err)
	}
	if shouldExit {
		return nil, nil
	}
	return workspaceInfo, nil
}

func (cmd *UpCmd) shouldPreventDaemonShutdown(workspaceInfo *provider2.AgentWorkspaceInfo) bool {
	return !workspaceInfo.CLIOptions.Platform.Enabled
}

func (cmd *UpCmd) shouldInstallDaemon(workspaceInfo *provider2.AgentWorkspaceInfo) bool {
	return !workspaceInfo.CLIOptions.Platform.Enabled && !workspaceInfo.CLIOptions.DisableDaemon
}

func (cmd *UpCmd) handleInitError(err error, workspaceInfo *provider2.AgentWorkspaceInfo, logger log.Logger) error {
	if logger == nil {
		logger = log.Discard
	}
	deleteErr := clientimplementation.DeleteWorkspaceFolder(clientimplementation.DeleteWorkspaceFolderParams{
		Context:              workspaceInfo.Workspace.Context,
		WorkspaceID:          workspaceInfo.Workspace.ID,
		SSHConfigPath:        workspaceInfo.Workspace.SSHConfigPath,
		SSHConfigIncludePath: workspaceInfo.Workspace.SSHConfigIncludePath,
	}, logger)
	if deleteErr != nil {
		return fmt.Errorf("%s: %w", deleteErr.Error(), err)
	}
	return err
}

func (cmd *UpCmd) cleanupCredentials(credentialsDir string) {
	if credentialsDir != "" {
		_ = os.RemoveAll(credentialsDir)
	}
}

func (cmd *UpCmd) up(ctx context.Context, workspaceInfo *provider2.AgentWorkspaceInfo, tunnelClient tunnel.TunnelClient, logger log.Logger) error {
	result, err := cmd.devPodUp(ctx, workspaceInfo, logger)
	if err != nil {
		return err
	}

	return cmd.sendResult(ctx, result, tunnelClient)
}

func (cmd *UpCmd) sendResult(ctx context.Context, result *config2.Result, tunnelClient tunnel.TunnelClient) error {
	out, err := json.Marshal(result)
	if err != nil {
		return err
	}

	_, err = tunnelClient.SendResult(ctx, &tunnel.Message{Message: string(out)})
	if err != nil {
		return fmt.Errorf("send result: %w", err)
	}

	return nil
}

func (cmd *UpCmd) devPodUp(ctx context.Context, workspaceInfo *provider2.AgentWorkspaceInfo, log log.Logger) (*config2.Result, error) {
	runner, err := CreateRunner(workspaceInfo, log)
	if err != nil {
		return nil, err
	}

	return runner.Up(ctx, devcontainer.UpOptions{
		CLIOptions:    workspaceInfo.CLIOptions,
		RegistryCache: workspaceInfo.RegistryCache,
	}, workspaceInfo.InjectTimeout)
}

func CreateRunner(workspaceInfo *provider2.AgentWorkspaceInfo, log log.Logger) (devcontainer.Runner, error) {
	return devcontainer.NewRunner(agent.ContainerDevPodHelperLocation, agent.DefaultAgentDownloadURL(), workspaceInfo, log)
}

func InitContentFolder(workspaceInfo *provider2.AgentWorkspaceInfo, log log.Logger) (bool, error) {
	exists, err := contentFolderExists(workspaceInfo.ContentFolder, log)
	if err != nil {
		return false, err
	}
	if exists {
		return true, nil
	}

	if err := createContentFolder(workspaceInfo.ContentFolder, log); err != nil {
		return false, err
	}

	if err := downloadWorkspaceBinaries(workspaceInfo, log); err != nil {
		_ = os.RemoveAll(workspaceInfo.ContentFolder)
		return false, err
	}

	if workspaceInfo.LastDevContainerConfig != nil {
		if err := ensureLastDevContainerJson(workspaceInfo); err != nil {
			log.Errorf("ensure devcontainer.json: %v", err)
		}
		return true, nil
	}

	return false, nil
}

func contentFolderExists(path string, log log.Logger) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		log.WithFields(logrus.Fields{"path": path}).Debug("workspace folder already exists")
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func createContentFolder(path string, log log.Logger) error {
	log.WithFields(logrus.Fields{"path": path}).Debug("create content folder")
	if err := os.MkdirAll(path, 0o777); err != nil {
		return fmt.Errorf("make workspace folder: %w", err)
	}
	return nil
}

func downloadWorkspaceBinaries(workspaceInfo *provider2.AgentWorkspaceInfo, log log.Logger) error {
	binariesDir, err := agent.GetAgentBinariesDir(
		workspaceInfo.Agent.DataPath,
		workspaceInfo.Workspace.Context,
		workspaceInfo.Workspace.ID,
	)
	if err != nil {
		return fmt.Errorf("error getting workspace %s binaries dir: %w", workspaceInfo.Workspace.ID, err)
	}

	_, err = binaries.DownloadBinaries(workspaceInfo.Agent.Binaries, binariesDir, log)
	if err != nil {
		return fmt.Errorf("error downloading workspace %s binaries: %w", workspaceInfo.Workspace.ID, err)
	}

	return nil
}

type workspaceInitializer struct {
	ctx                  context.Context
	cancel               context.CancelFunc
	workspaceInfo        *provider2.AgentWorkspaceInfo
	debug                bool
	shouldInstallDaemon  bool
	tunnelClient         tunnel.TunnelClient
	logger               log.Logger
	dockerCredentialsDir string
	gitCredentialsHelper string
}

func initWorkspace(ctx context.Context, cancel context.CancelFunc, workspaceInfo *provider2.AgentWorkspaceInfo, debug, shouldInstallDaemon bool) (tunnel.TunnelClient, log.Logger, string, error) {
	init := &workspaceInitializer{
		ctx:                 ctx,
		cancel:              cancel,
		workspaceInfo:       workspaceInfo,
		debug:               debug,
		shouldInstallDaemon: shouldInstallDaemon,
	}

	if err := init.initializeTunnel(); err != nil {
		return nil, nil, "", err
	}

	if err := init.setupCredentials(); err != nil {
		init.logger.Errorf("error retrieving docker / git credentials: %v", err)
	}

	dockerErrChan := init.installDockerAsync()

	if err := init.prepareWorkspaceContent(); err != nil {
		return nil, init.logger, init.dockerCredentialsDir, err
	}

	if init.shouldInstallDaemon {
		if err := installDaemon(init.workspaceInfo, init.logger); err != nil {
			init.logger.Errorf("install DevPod daemon: %v", err)
		}
	}

	if err := init.waitForDocker(dockerErrChan); err != nil {
		return nil, nil, init.dockerCredentialsDir, err
	}

	daemonErrChan := init.configureDockerDaemonAsync()
	if err := <-daemonErrChan; err != nil {
		init.logger.Warn(
			"could not find docker daemon config file, if using the registry cache, " +
				"please ensure the daemon is configured with containerd-snapshotter=true, " +
				"more info at https://docs.docker.com/engine/storage/containerd/",
		)
	}

	return init.tunnelClient, init.logger, init.dockerCredentialsDir, nil
}

func (w *workspaceInitializer) initializeTunnel() error {
	client, err := tunnelserver.NewTunnelClient(os.Stdin, os.Stdout, true, 0)
	if err != nil {
		return fmt.Errorf("error creating tunnel client: %w", err)
	}
	w.tunnelClient = client
	w.logger = tunnelserver.NewTunnelLogger(w.ctx, w.tunnelClient, w.debug)
	w.logger.Debugf("created logger")

	if _, err := w.tunnelClient.Ping(w.ctx, &tunnel.Empty{}); err != nil {
		return fmt.Errorf("ping client: %w", err)
	}

	return nil
}

func (w *workspaceInitializer) setupCredentials() error {
	dockerCredentialsDir, gitCredentialsHelper, err := configureCredentials(credentialsConfig{
		ctx:           w.ctx,
		workspaceInfo: w.workspaceInfo,
		client:        w.tunnelClient,
		log:           w.logger,
	})
	w.dockerCredentialsDir = dockerCredentialsDir
	w.gitCredentialsHelper = gitCredentialsHelper
	return err
}

type dockerInstallResult struct {
	path string
	err  error
}

func (w *workspaceInitializer) installDockerAsync() <-chan dockerInstallResult {
	resultChan := make(chan dockerInstallResult, 1)

	go func() {
		if !w.workspaceInfo.Agent.IsDockerDriver() {
			w.logger.Debug("not a docker driver, skipping docker installation")
			resultChan <- dockerInstallResult{}
			return
		}

		dockerPath, err := w.ensureDockerInstalled()
		resultChan <- dockerInstallResult{path: dockerPath, err: err}
	}()

	return resultChan
}

func (w *workspaceInitializer) ensureDockerInstalled() (string, error) {
	dockerCmd := w.getDockerCommand()

	if command.Exists(dockerCmd) {
		w.logger.Debug("docker command exists, skipping installation")
		return "", nil
	}

	if dockerCmd != "docker" {
		_, err := exec.LookPath(dockerCmd)
		return "", err
	}

	if w.isDockerInstallDisabled() {
		w.logger.Debug("docker not found but installation was disabled, installing anyway as it is required")
	}

	w.logger.Debug("attempting to install docker")
	dockerPath, err := installDocker(w.logger)
	w.logger.Debugf("docker installation path=%q, err=%v", dockerPath, err)
	return dockerPath, err
}

func (w *workspaceInitializer) getDockerCommand() string {
	if w.workspaceInfo.Agent.Docker.Path != "" {
		w.logger.Debugf("using custom docker path %s", w.workspaceInfo.Agent.Docker.Path)
		return w.workspaceInfo.Agent.Docker.Path
	}
	return "docker"
}

func (w *workspaceInitializer) isDockerInstallDisabled() bool {
	install, err := w.workspaceInfo.Agent.Docker.Install.Bool()
	return err == nil && !install
}

func (w *workspaceInitializer) prepareWorkspaceContent() error {
	return prepareWorkspace(prepareWorkspaceParams{
		ctx:           w.ctx,
		workspaceInfo: w.workspaceInfo,
		client:        w.tunnelClient,
		gitHelper:     w.gitCredentialsHelper,
		log:           w.logger,
	})
}

func (w *workspaceInitializer) waitForDocker(resultChan <-chan dockerInstallResult) error {
	result := <-resultChan

	if result.path != "" && w.workspaceInfo.Agent.Docker.Path == "" {
		w.workspaceInfo.Agent.Docker.Path = result.path
		w.logger.Debugf("set docker path to %s", result.path)
	}

	if result.err != nil {
		return fmt.Errorf("install docker: %w", result.err)
	}

	return nil
}

func (w *workspaceInitializer) configureDockerDaemonAsync() <-chan error {
	errChan := make(chan error, 1)

	if !w.shouldConfigureDockerDaemon() {
		w.logger.Debug("skipping configuring docker daemon")
		errChan <- nil
		return errChan
	}

	go func() {
		errChan <- configureDockerDaemon(w.ctx, w.logger)
	}()

	return errChan
}

func (w *workspaceInitializer) shouldConfigureDockerDaemon() bool {
	if !w.workspaceInfo.Agent.IsDockerDriver() {
		return false
	}

	local, err := w.workspaceInfo.Agent.Local.Bool()
	if err != nil {
		w.logger.Debugf("failed to parse Local option: %v", err)
		return false
	}
	return !local
}

type prepareWorkspaceParams struct {
	ctx           context.Context
	workspaceInfo *provider2.AgentWorkspaceInfo
	client        tunnel.TunnelClient
	gitHelper     string
	log           log.Logger
}

func prepareWorkspace(params prepareWorkspaceParams) error {
	if params.workspaceInfo.CLIOptions.Platform.Enabled && params.workspaceInfo.Workspace.Source.LocalFolder != "" {
		params.workspaceInfo.ContentFolder = agent.GetAgentWorkspaceContentDir(params.workspaceInfo.Origin)
	}

	exists, err := InitContentFolder(params.workspaceInfo, params.log)
	if err != nil {
		return err
	}
	if exists && !params.workspaceInfo.CLIOptions.Recreate {
		params.log.Debugf("workspace exists, skip downloading")
		return nil
	}

	if params.workspaceInfo.Workspace.Source.GitRepository != "" {
		return prepareGitWorkspace(prepareGitWorkspaceParams{
			ctx:           params.ctx,
			workspaceInfo: params.workspaceInfo,
			gitHelper:     params.gitHelper,
			exists:        exists,
			log:           params.log,
		})
	}

	if params.workspaceInfo.Workspace.Source.LocalFolder != "" {
		return prepareLocalWorkspace(params.ctx, params.workspaceInfo, params.client, params.log)
	}

	if params.workspaceInfo.Workspace.Source.Image != "" {
		params.log.Debugf("prepare image")
		return prepareImage(params.workspaceInfo.ContentFolder, params.workspaceInfo.Workspace.Source.Image)
	}

	if params.workspaceInfo.Workspace.Source.Container != "" {
		params.log.Debugf("workspace is a container, nothing to do")
		return nil
	}

	return fmt.Errorf("either workspace repository, image, container or local-folder is required")
}

type prepareGitWorkspaceParams struct {
	ctx           context.Context
	workspaceInfo *provider2.AgentWorkspaceInfo
	gitHelper     string
	exists        bool
	log           log.Logger
}

func prepareGitWorkspace(params prepareGitWorkspaceParams) error {
	if params.workspaceInfo.CLIOptions.Reset {
		params.log.Info("resetting git based workspace, removing old content folder")
		if err := os.RemoveAll(params.workspaceInfo.ContentFolder); err != nil {
			params.log.Warnf("failed to remove workspace folder, still proceeding: %v", err)
		}
	}

	if params.workspaceInfo.CLIOptions.Recreate && !params.workspaceInfo.CLIOptions.Reset && params.exists {
		params.log.Info("rebuilding without resetting a git based workspace, keeping old content folder")
		return nil
	}

	if crane.ShouldUse(&params.workspaceInfo.CLIOptions) {
		params.log.Infof("pulling devcontainer spec from %v", params.workspaceInfo.CLIOptions.Platform.EnvironmentTemplate)
		return nil
	}

	return agent.CloneRepositoryForWorkspace(
		params.ctx,
		&params.workspaceInfo.Workspace.Source,
		&params.workspaceInfo.Agent,
		params.workspaceInfo.ContentFolder,
		params.gitHelper,
		params.workspaceInfo.CLIOptions,
		false,
		params.log,
	)
}

func prepareLocalWorkspace(ctx context.Context, workspaceInfo *provider2.AgentWorkspaceInfo, client tunnel.TunnelClient, log log.Logger) error {
	if workspaceInfo.ContentFolder == workspaceInfo.Workspace.Source.LocalFolder {
		log.Debugf("local folder %s with local provider; skip downloading", workspaceInfo.ContentFolder)
		return nil
	}

	log.Debugf("download local folder %s", workspaceInfo.ContentFolder)
	return downloadLocalFolder(ctx, workspaceInfo.ContentFolder, client, log)
}

func ensureLastDevContainerJson(workspaceInfo *provider2.AgentWorkspaceInfo) error {
	filePath := filepath.Join(workspaceInfo.ContentFolder, filepath.FromSlash(workspaceInfo.LastDevContainerConfig.Path))

	if _, err := os.Stat(filePath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("error stating %s: %w", filePath, err)
	}

	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(filePath), err)
	}

	raw, err := json.Marshal(workspaceInfo.LastDevContainerConfig.Config)
	if err != nil {
		return fmt.Errorf("marshal devcontainer.json: %w", err)
	}

	if err := os.WriteFile(filePath, raw, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", filePath, err)
	}

	return nil
}

type credentialsConfig struct {
	ctx           context.Context
	workspaceInfo *provider2.AgentWorkspaceInfo
	client        tunnel.TunnelClient
	log           log.Logger
}

func configureCredentials(cfg credentialsConfig) (string, string, error) {
	if cfg.workspaceInfo.Agent.InjectDockerCredentials != "true" && cfg.workspaceInfo.Agent.InjectGitCredentials != "true" {
		return "", "", nil
	}

	serverPort, err := credentials.StartCredentialsServer(cfg.ctx, cfg.client, cfg.log)
	if err != nil {
		return "", "", err
	}

	binaryPath, err := os.Executable()
	if err != nil {
		return "", "", err
	}

	if cfg.workspaceInfo.Origin == "" {
		return "", "", fmt.Errorf("workspace folder is not set")
	}

	dockerCredentials := ""
	if cfg.workspaceInfo.Agent.InjectDockerCredentials == "true" {
		dockerCredentials, err = dockercredentials.ConfigureCredentialsMachine(cfg.workspaceInfo.Origin, serverPort, cfg.log)
		if err != nil {
			return "", "", err
		}
	}

	gitCredentials := ""
	if cfg.workspaceInfo.Agent.InjectGitCredentials == "true" {
		gitCredentials = fmt.Sprintf("!'%s' agent git-credentials --port %d", binaryPath, serverPort)
		_ = os.Setenv("DEVPOD_GIT_HELPER_PORT", strconv.Itoa(serverPort))
	}

	return dockerCredentials, gitCredentials, nil
}

func installDaemon(workspaceInfo *provider2.AgentWorkspaceInfo, log log.Logger) error {
	if len(workspaceInfo.Agent.Exec.Shutdown) == 0 {
		return nil
	}

	log.Debugf("installing DevPod daemon into server")
	return agentdaemon.InstallDaemon(workspaceInfo.Agent.DataPath, workspaceInfo.CLIOptions.DaemonInterval, log)
}

func downloadLocalFolder(ctx context.Context, workspaceDir string, client tunnel.TunnelClient, log log.Logger) error {
	log.Infof("Upload folder to server")
	stream, err := client.StreamWorkspace(ctx, &tunnel.Empty{})
	if err != nil {
		return fmt.Errorf("read workspace: %w", err)
	}

	return extract.Extract(tunnelserver.NewStreamReader(stream, log), workspaceDir)
}

func prepareImage(workspaceDir, image string) error {
	devcontainerConfig := []byte(`{
  "image": "` + image + `"
}`)
	return os.WriteFile(filepath.Join(workspaceDir, ".devcontainer.json"), devcontainerConfig, 0o600)
}

func installDocker(log log.Logger) (dockerPath string, err error) {
	if !command.Exists("docker") {
		writer := log.Writer(logrus.InfoLevel, false)
		defer func() { _ = writer.Close() }()

		log.Debug("Installing Docker")

		dockerPath, err = dockerinstall.Install(writer, writer)
	} else {
		dockerPath = "docker"
	}
	return dockerPath, err
}

func configureDockerDaemon(ctx context.Context, log log.Logger) error {
	log.Info("configuring docker daemon")

	daemonConfig := []byte(`{
		"features": {
			"containerd-snapshotter": true
		}
	}`)

	if err := writeDockerDaemonConfig(daemonConfig); err != nil {
		return err
	}

	return reloadDockerDaemon(ctx)
}

func writeDockerDaemonConfig(config []byte) error {
	if err := tryWriteRootlessDockerConfig(config); err == nil {
		return nil
	}

	return os.WriteFile("/etc/docker/daemon.json", config, 0644)
}

func tryWriteRootlessDockerConfig(config []byte) error {
	homeDir, err := util.UserHomeDir()
	if err != nil {
		return err
	}

	dockerConfigDir := filepath.Join(homeDir, ".config", "docker")
	if _, err := os.Stat(dockerConfigDir); errors.Is(err, os.ErrNotExist) {
		return err
	}

	return os.WriteFile(filepath.Join(dockerConfigDir, "daemon.json"), config, 0644)
}

func reloadDockerDaemon(ctx context.Context) error {
	return exec.CommandContext(ctx, "pkill", "-HUP", "dockerd").Run()
}
