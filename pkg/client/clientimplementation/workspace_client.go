package clientimplementation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/gofrs/flock"
	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/pkg/agent"
	"github.com/skevetter/devpod/pkg/agent/tunnelserver"
	"github.com/skevetter/devpod/pkg/binaries"
	"github.com/skevetter/devpod/pkg/client"
	"github.com/skevetter/devpod/pkg/compress"
	"github.com/skevetter/devpod/pkg/config"
	config2 "github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/skevetter/devpod/pkg/options"
	"github.com/skevetter/devpod/pkg/provider"
	"github.com/skevetter/devpod/pkg/shell"
	"github.com/skevetter/devpod/pkg/ssh"
	"github.com/skevetter/devpod/pkg/types"
	"github.com/skevetter/log"
)

func NewWorkspaceClient(devPodConfig *config.Config, prov *provider.ProviderConfig, workspace *provider.Workspace, machine *provider.Machine, log log.Logger) (client.WorkspaceClient, error) {
	if workspace.Machine.ID != "" && machine == nil {
		return nil, fmt.Errorf("workspace machine is not found")
	} else if prov.IsMachineProvider() && workspace.Machine.ID == "" {
		return nil, fmt.Errorf("workspace machine ID is empty, but machine provider found")
	}

	return &workspaceClient{
		devPodConfig: devPodConfig,
		config:       prov,
		workspace:    workspace,
		machine:      machine,
		log:          log,
	}, nil
}

type workspaceClient struct {
	m sync.Mutex

	workspaceLockOnce sync.Once
	workspaceLock     *flock.Flock
	machineLock       *flock.Flock

	devPodConfig *config.Config
	config       *provider.ProviderConfig
	workspace    *provider.Workspace
	machine      *provider.Machine
	log          log.Logger
}

func (s *workspaceClient) Provider() string {
	return s.config.Name
}

func (s *workspaceClient) Workspace() string {
	s.m.Lock()
	defer s.m.Unlock()

	return s.workspace.ID
}

func (s *workspaceClient) WorkspaceConfig() *provider.Workspace {
	s.m.Lock()
	defer s.m.Unlock()

	return provider.CloneWorkspace(s.workspace)
}

func (s *workspaceClient) AgentLocal() bool {
	s.m.Lock()
	defer s.m.Unlock()

	return options.ResolveAgentConfig(s.devPodConfig, s.config, s.workspace, s.machine).Local == "true"
}

func (s *workspaceClient) AgentPath() string {
	s.m.Lock()
	defer s.m.Unlock()

	return options.ResolveAgentConfig(s.devPodConfig, s.config, s.workspace, s.machine).Path
}

func (s *workspaceClient) AgentURL() string {
	s.m.Lock()
	defer s.m.Unlock()

	return options.ResolveAgentConfig(s.devPodConfig, s.config, s.workspace, s.machine).DownloadURL
}

func (s *workspaceClient) Context() string {
	return s.workspace.Context
}

func (s *workspaceClient) RefreshOptions(ctx context.Context, userOptionsRaw []string, reconfigure bool) error {
	s.m.Lock()
	defer s.m.Unlock()

	userOptions, err := provider.ParseOptions(userOptionsRaw)
	if err != nil {
		return fmt.Errorf("parse options %w", err)
	}

	if s.isMachineProvider() {
		if s.machine == nil {
			return nil
		}

		machine, err := options.ResolveAndSaveOptionsMachine(ctx, s.devPodConfig, s.config, s.machine, userOptions, s.log)
		if err != nil {
			return err
		}

		s.machine = machine
		return nil
	}

	workspace, err := options.ResolveAndSaveOptionsWorkspace(ctx, s.devPodConfig, s.config, s.workspace, userOptions, s.log)
	if err != nil {
		s.log.WithFields(logrus.Fields{
			"error": err,
		}).Error("failed to resolve and save options workspace")
		return err
	}

	if workspace != nil {
		s.workspace = workspace
		s.log.WithFields(logrus.Fields{
			"workspaceId": s.workspace.ID,
		}).Debug("refreshed workspace options")
	} else {
		s.log.Debug("workspace is nil; not updating workspace options")
	}
	return nil
}

func (s *workspaceClient) AgentInjectGitCredentials(cliOptions provider.CLIOptions) bool {
	s.m.Lock()
	defer s.m.Unlock()

	return s.agentInfo(cliOptions).Agent.InjectGitCredentials == "true"
}

func (s *workspaceClient) AgentInjectDockerCredentials(cliOptions provider.CLIOptions) bool {
	s.m.Lock()
	defer s.m.Unlock()

	return s.agentInfo(cliOptions).Agent.InjectDockerCredentials == "true"
}

func (s *workspaceClient) AgentInfo(cliOptions provider.CLIOptions) (string, *provider.AgentWorkspaceInfo, error) {
	s.m.Lock()
	defer s.m.Unlock()

	return s.compressedAgentInfo(cliOptions)
}

func (s *workspaceClient) compressedAgentInfo(cliOptions provider.CLIOptions) (string, *provider.AgentWorkspaceInfo, error) {
	agentInfo := s.agentInfo(cliOptions)

	// marshal config
	out, err := json.Marshal(agentInfo)
	if err != nil {
		return "", nil, err
	}

	compressed, err := compress.Compress(string(out))
	if err != nil {
		return "", nil, err
	}

	return compressed, agentInfo, nil
}

func (s *workspaceClient) agentInfo(cliOptions provider.CLIOptions) *provider.AgentWorkspaceInfo {
	// try to load last devcontainer.json
	var lastDevContainerConfig *config2.DevContainerConfigWithPath
	var workspaceOrigin string
	if s.workspace != nil {
		result, err := provider.LoadWorkspaceResult(s.workspace.Context, s.workspace.ID)
		if err != nil {
			s.log.WithFields(logrus.Fields{"error": err}).Debug("error loading workspace result")
		} else if result != nil {
			lastDevContainerConfig = result.DevContainerConfigWithPath
		}

		workspaceOrigin = s.workspace.Origin
	}

	// build struct
	agentInfo := &provider.AgentWorkspaceInfo{
		WorkspaceOrigin:        workspaceOrigin,
		Workspace:              s.workspace,
		Machine:                s.machine,
		LastDevContainerConfig: lastDevContainerConfig,
		CLIOptions:             cliOptions,
		Agent:                  options.ResolveAgentConfig(s.devPodConfig, s.config, s.workspace, s.machine),
		Options:                s.devPodConfig.ProviderOptions(s.Provider()),
	}

	// if we are running platform mode
	if cliOptions.Platform.Enabled {
		agentInfo.Agent.InjectGitCredentials = "true"
		agentInfo.Agent.InjectDockerCredentials = "true"
	}

	// we don't send any provider options if proxy because these could contain
	// sensitive information and we don't want to allow privileged containers that
	// have access to the host to save these.
	if agentInfo.Agent.Driver != provider.CustomDriver && (cliOptions.Platform.Enabled || cliOptions.DisableDaemon) {
		agentInfo.Options = map[string]config.OptionValue{}
		agentInfo.Workspace = provider.CloneWorkspace(agentInfo.Workspace)
		agentInfo.Workspace.Provider.Options = map[string]config.OptionValue{}
		if agentInfo.Machine != nil {
			agentInfo.Machine = provider.CloneMachine(agentInfo.Machine)
			agentInfo.Machine.Provider.Options = map[string]config.OptionValue{}
		}
	}

	// Get the timeout from the context options
	agentInfo.InjectTimeout = config.ParseTimeOption(s.devPodConfig, config.ContextOptionAgentInjectTimeout)

	// Set registry cache from context option
	agentInfo.RegistryCache = s.devPodConfig.ContextOption(config.ContextOptionRegistryCache)

	return agentInfo
}

func (s *workspaceClient) initLock() {
	s.workspaceLockOnce.Do(func() {
		s.m.Lock()
		defer s.m.Unlock()

		// get locks dir
		workspaceLocksDir, err := provider.GetLocksDir(s.workspace.Context)
		if err != nil {
			panic(fmt.Errorf("get workspaces dir %w", err))
		}
		_ = os.MkdirAll(workspaceLocksDir, 0777)

		// create workspace lock
		s.workspaceLock = flock.New(filepath.Join(workspaceLocksDir, s.workspace.ID+".workspace.lock"))

		// create machine lock
		if s.machine != nil {
			s.machineLock = flock.New(filepath.Join(workspaceLocksDir, s.machine.ID+".machine.lock"))
		}
	})
}

func (s *workspaceClient) Lock(ctx context.Context) error {
	s.initLock()

	// try to lock workspace
	s.log.Debug("acquire workspace lock")
	err := tryLock(ctx, s.workspaceLock, "workspace", s.log)
	if err != nil {
		return fmt.Errorf("error locking workspace %w", err)
	}
	s.log.Debug("acquired workspace lock")

	// try to lock machine
	if s.machineLock != nil {
		s.log.Debug("acquire machine lock")
		err := tryLock(ctx, s.machineLock, "machine", s.log)
		if err != nil {
			return fmt.Errorf("error locking machine %w", err)
		}
		s.log.Debug("acquired machine lock")
	}

	return nil
}

func (s *workspaceClient) Unlock() {
	s.initLock()

	// try to unlock machine
	if s.machineLock != nil {
		err := s.machineLock.Unlock()
		if err != nil {
			s.log.WithFields(logrus.Fields{"error": err}).Warn("error unlocking machine")
		}
	}

	// try to unlock workspace
	err := s.workspaceLock.Unlock()
	if err != nil {
		s.log.WithFields(logrus.Fields{"error": err}).Warn("error unlocking workspace")
	}
}

func (s *workspaceClient) Create(ctx context.Context, options client.CreateOptions) error {
	s.m.Lock()
	defer s.m.Unlock()

	// provider doesn't support machines
	if !s.isMachineProvider() {
		return nil
	}

	// check machine state
	if s.machine == nil {
		return fmt.Errorf("machine is not defined")
	}

	// create machine client
	machineClient, err := NewMachineClient(s.devPodConfig, s.config, s.machine, s.log)
	if err != nil {
		return err
	}

	// get status
	machineStatus, err := machineClient.Status(ctx, client.StatusOptions{})
	if err != nil {
		return err
	} else if machineStatus != client.StatusNotFound {
		return nil
	}

	// create the machine
	return machineClient.Create(ctx, client.CreateOptions{})
}

func (s *workspaceClient) Delete(ctx context.Context, opt client.DeleteOptions) error {
	s.m.Lock()
	defer s.m.Unlock()

	// parse duration
	var gracePeriod *time.Duration
	if opt.GracePeriod != "" {
		duration, err := time.ParseDuration(opt.GracePeriod)
		if err == nil {
			gracePeriod = &duration
		}
	}

	// kill the command after the grace period
	if gracePeriod != nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, *gracePeriod)
		defer cancel()
	}

	// should just delete container?
	if !s.isMachineProvider() || !s.workspace.Machine.AutoDelete {
		isRunning, err := s.isMachineRunning(ctx)
		if err != nil {
			if !opt.Force {
				return err
			}
		} else if isRunning {
			writer := s.log.Writer(logrus.InfoLevel, false)
			defer func() { _ = writer.Close() }()

			s.log.Info("deleting workspace container")
			compressed, info, err := s.compressedAgentInfo(provider.CLIOptions{})
			if err != nil {
				return fmt.Errorf("agent info")
			}
			command := fmt.Sprintf("'%s' agent workspace delete --workspace-info '%s'", info.Agent.Path, compressed)
			err = RunCommandWithBinaries(CommandOptions{
				Ctx:       ctx,
				Name:      "command",
				Command:   s.config.Exec.Command,
				Context:   s.workspace.Context,
				Workspace: s.workspace,
				Machine:   s.machine,
				Options:   s.devPodConfig.ProviderOptions(s.config.Name),
				Config:    s.config,
				ExtraEnv: map[string]string{
					provider.CommandEnv: command,
				},
				Stdin:  nil,
				Stdout: writer,
				Stderr: writer,
				Log:    s.log.ErrorStreamOnly(),
			})
			if err != nil {
				if !opt.Force {
					return err
				}

				if !errors.Is(err, context.DeadlineExceeded) {
					s.log.WithFields(logrus.Fields{"error": err}).Error("error deleting container")
				}
			}
		}
	} else if s.machine != nil && s.workspace.Machine.ID != "" && len(s.config.Exec.Delete) > 0 {
		// delete machine if config was found
		machineClient, err := NewMachineClient(s.devPodConfig, s.config, s.machine, s.log)
		if err != nil {
			if !opt.Force {
				return err
			}
		}

		err = machineClient.Delete(ctx, opt)
		if err != nil {
			return err
		}
	}

	return DeleteWorkspaceFolder(DeleteWorkspaceFolderParams{
		Context:              s.workspace.Context,
		WorkspaceID:          s.workspace.ID,
		SSHConfigPath:        s.workspace.SSHConfigPath,
		SSHConfigIncludePath: s.workspace.SSHConfigIncludePath,
	}, s.log)
}

func (s *workspaceClient) isMachineRunning(ctx context.Context) (bool, error) {
	if !s.isMachineProvider() {
		return true, nil
	}

	// delete machine if config was found
	machineClient, err := NewMachineClient(s.devPodConfig, s.config, s.machine, s.log)
	if err != nil {
		return false, err
	}

	// retrieve status
	status, err := machineClient.Status(ctx, client.StatusOptions{})
	if err != nil {
		return false, fmt.Errorf("retrieve machine status %w", err)
	} else if status == client.StatusRunning {
		return true, nil
	}

	return false, nil
}

func (s *workspaceClient) Start(ctx context.Context, options client.StartOptions) error {
	s.m.Lock()
	defer s.m.Unlock()

	if !s.isMachineProvider() || s.machine == nil {
		return nil
	}

	machineClient, err := NewMachineClient(s.devPodConfig, s.config, s.machine, s.log)
	if err != nil {
		return err
	}

	return machineClient.Start(ctx, options)
}

func (s *workspaceClient) Stop(ctx context.Context, opt client.StopOptions) error {
	s.m.Lock()
	defer s.m.Unlock()

	if !s.isMachineProvider() || !s.workspace.Machine.AutoDelete {
		writer := s.log.Writer(logrus.InfoLevel, false)
		defer func() { _ = writer.Close() }()

		s.log.Info("stopping container")
		compressed, info, err := s.compressedAgentInfo(provider.CLIOptions{})
		if err != nil {
			return fmt.Errorf("agent info")
		}
		command := fmt.Sprintf("'%s' agent workspace stop --workspace-info '%s'", info.Agent.Path, compressed)
		err = RunCommandWithBinaries(CommandOptions{
			Ctx:       ctx,
			Name:      "command",
			Command:   s.config.Exec.Command,
			Context:   s.workspace.Context,
			Workspace: s.workspace,
			Machine:   s.machine,
			Options:   s.devPodConfig.ProviderOptions(s.config.Name),
			Config:    s.config,
			ExtraEnv: map[string]string{
				provider.CommandEnv: command,
			},
			Stdin:  nil,
			Stdout: writer,
			Stderr: writer,
			Log:    s.log.ErrorStreamOnly(),
		})
		if err != nil {
			return err
		}
		s.log.Info("stopped container")

		return nil
	}

	machineClient, err := NewMachineClient(s.devPodConfig, s.config, s.machine, s.log)
	if err != nil {
		return err
	}

	return machineClient.Stop(ctx, opt)
}

func (s *workspaceClient) Command(ctx context.Context, commandOptions client.CommandOptions) (err error) {
	// get environment variables
	s.m.Lock()
	environ, err := binaries.ToEnvironmentWithBinaries(binaries.EnvironmentOptions{
		Context:   s.workspace.Context,
		Workspace: s.workspace,
		Machine:   s.machine,
		Options:   s.devPodConfig.ProviderOptions(s.config.Name),
		Config:    s.config,
		ExtraEnv: map[string]string{
			provider.CommandEnv: commandOptions.Command,
		},
		Log: s.log,
	})
	if err != nil {
		return err
	}
	s.m.Unlock()

	return RunCommand(RunCommandOptions{
		Ctx:     ctx,
		Command: s.config.Exec.Command,
		Environ: environ,
		Stdin:   commandOptions.Stdin,
		Stdout:  commandOptions.Stdout,
		Stderr:  commandOptions.Stderr,
		Log:     s.log.ErrorStreamOnly(),
	})
}

func (s *workspaceClient) Status(ctx context.Context, options client.StatusOptions) (client.Status, error) {
	s.m.Lock()
	defer s.m.Unlock()

	// check if provider has status command
	if s.isMachineProvider() && len(s.config.Exec.Status) > 0 {
		if s.machine == nil {
			return client.StatusNotFound, nil
		}

		machineClient, err := NewMachineClient(s.devPodConfig, s.config, s.machine, s.log)
		if err != nil {
			return client.StatusNotFound, err
		}

		status, err := machineClient.Status(ctx, options)
		if err != nil {
			return status, err
		}

		// try to check container status and if that fails check workspace folder
		if status == client.StatusRunning && options.ContainerStatus {
			return s.getContainerStatus(ctx)
		}

		return status, err
	}

	// try to check container status and if that fails check workspace folder
	if options.ContainerStatus {
		return s.getContainerStatus(ctx)
	}

	// logic:
	// - if workspace folder exists -> Running
	// - if workspace folder doesn't exist -> NotFound
	workspaceFolder, err := provider.GetWorkspaceDir(s.workspace.Context, s.workspace.ID)
	if err != nil {
		return "", err
	}

	// does workspace folder exist?
	_, err = os.Stat(workspaceFolder)
	if err == nil {
		return client.StatusRunning, nil
	}

	return client.StatusNotFound, nil
}

func (s *workspaceClient) getContainerStatus(ctx context.Context) (client.Status, error) {
	stdout := &bytes.Buffer{}
	buf := &bytes.Buffer{}
	compressed, info, err := s.compressedAgentInfo(provider.CLIOptions{})
	if err != nil {
		return "", fmt.Errorf("get agent info")
	}
	command := fmt.Sprintf("'%s' agent workspace status --workspace-info '%s'", info.Agent.Path, compressed)
	err = RunCommandWithBinaries(CommandOptions{
		Ctx:       ctx,
		Name:      "command",
		Command:   s.config.Exec.Command,
		Context:   s.workspace.Context,
		Workspace: s.workspace,
		Machine:   s.machine,
		Options:   s.devPodConfig.ProviderOptions(s.config.Name),
		Config:    s.config,
		ExtraEnv: map[string]string{
			provider.CommandEnv: command,
		},
		Stdin:  nil,
		Stdout: io.MultiWriter(stdout, buf),
		Stderr: buf,
		Log:    s.log.ErrorStreamOnly(),
	})
	if err != nil {
		return client.StatusNotFound, fmt.Errorf("error retrieving container status: %s%w", buf.String(), err)
	}

	parsed, err := client.ParseStatus(stdout.String())
	if err != nil {
		return client.StatusNotFound, fmt.Errorf("error parsing container status: %s%w", buf.String(), err)
	}

	s.log.WithFields(logrus.Fields{
		"stdout": buf.String(),
		"stderr": stdout.String(),
		"parsed": parsed,
	}).Debug("container status command output")
	return parsed, nil
}

func (s *workspaceClient) isMachineProvider() bool {
	return len(s.config.Exec.Create) > 0
}

type CommandOptions struct {
	Ctx       context.Context
	Name      string
	Command   types.StrArray
	Context   string
	Workspace *provider.Workspace
	Machine   *provider.Machine
	Options   map[string]config.OptionValue
	Config    *provider.ProviderConfig
	ExtraEnv  map[string]string
	Stdin     io.Reader
	Stdout    io.Writer
	Stderr    io.Writer
	Log       log.Logger
}

func RunCommandWithBinaries(opts CommandOptions) error {
	environ, err := binaries.ToEnvironmentWithBinaries(binaries.EnvironmentOptions{
		Context:   opts.Context,
		Workspace: opts.Workspace,
		Machine:   opts.Machine,
		Options:   opts.Options,
		Config:    opts.Config,
		ExtraEnv:  opts.ExtraEnv,
		Log:       opts.Log,
	})
	if err != nil {
		return err
	}

	return RunCommand(RunCommandOptions{
		Ctx:     opts.Ctx,
		Command: opts.Command,
		Environ: environ,
		Stdin:   opts.Stdin,
		Stdout:  opts.Stdout,
		Stderr:  opts.Stderr,
		Log:     opts.Log,
	})
}

type RunCommandOptions struct {
	Ctx     context.Context
	Command types.StrArray
	Environ []string
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
	Log     log.Logger // Optional: for debug mode env var
}

func RunCommand(opts RunCommandOptions) error {
	if len(opts.Command) == 0 {
		return nil
	}

	// Add debug env var if logger provided and in debug mode
	if opts.Log != nil && opts.Log.GetLevel() == logrus.DebugLevel {
		opts.Environ = append(opts.Environ, DevPodDebug+"=true")
	}

	// use shell if command length is equal 1
	if len(opts.Command) == 1 {
		return shell.RunEmulatedShell(opts.Ctx, opts.Command[0], opts.Stdin, opts.Stdout, opts.Stderr, opts.Environ)
	}

	// run command
	cmd := exec.CommandContext(opts.Ctx, opts.Command[0], opts.Command[1:]...)
	cmd.Stdin = opts.Stdin
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	cmd.Env = opts.Environ
	err := cmd.Run()
	if err != nil {
		return err
	}

	return nil
}

func DeleteMachineFolder(context, machineID string) error {
	machineDir, err := provider.GetMachineDir(context, machineID)
	if err != nil {
		return err
	}

	// remove machine folder
	err = os.RemoveAll(machineDir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

type DeleteWorkspaceFolderParams struct {
	Context              string
	WorkspaceID          string
	SSHConfigPath        string
	SSHConfigIncludePath string
}

func DeleteWorkspaceFolder(params DeleteWorkspaceFolderParams, log log.Logger) error {
	path, err := ssh.ResolveSSHConfigPath(params.SSHConfigPath)
	if err != nil {
		return err
	}
	sshConfigPath := path

	sshConfigIncludePath := params.SSHConfigIncludePath
	if sshConfigIncludePath != "" {
		includePath, err := ssh.ResolveSSHConfigPath(sshConfigIncludePath)
		if err != nil {
			return err
		}
		sshConfigIncludePath = includePath
	}

	err = ssh.RemoveFromConfig(params.WorkspaceID, sshConfigPath, sshConfigIncludePath, log)
	if err != nil {
		log.Errorf("Remove workspace '%s' from ssh config: %v", params.WorkspaceID, err)
	}

	workspaceFolder, err := provider.GetWorkspaceDir(params.Context, params.WorkspaceID)
	if err != nil {
		return err
	}

	// remove workspace folder
	err = os.RemoveAll(workspaceFolder)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

const (
	pollInterval = 2 * time.Second
	logThreshold = 10 * time.Second
)

// StartWait waits for the workspace to be ready, optionally creating/starting it
func StartWait(
	ctx context.Context,
	workspaceClient client.WorkspaceClient,
	create bool,
	log log.Logger,
) error {
	startWaiting := time.Now()
	for {
		instanceStatus, err := workspaceClient.Status(ctx, client.StatusOptions{})
		if err != nil {
			return err
		}

		switch instanceStatus {
		case client.StatusBusy:
			if handleBusyStatus(&startWaiting, log) {
				time.Sleep(pollInterval)
				continue
			}
		case client.StatusStopped:
			if err := handleStoppedStatus(ctx, workspaceClient, create); err != nil {
				return err
			}
		case client.StatusNotFound:
			if err := handleNotFoundStatus(ctx, workspaceClient, create); err != nil {
				return err
			}
		default:
			return nil
		}
	}
}

func handleBusyStatus(startWaiting *time.Time, log log.Logger) bool {
	if time.Since(*startWaiting) > logThreshold {
		log.Info("workspace is busy, waiting for workspace to become ready")
		*startWaiting = time.Now()
	}
	return true
}

func handleStoppedStatus(ctx context.Context, workspaceClient client.WorkspaceClient, create bool) error {
	if create {
		err := workspaceClient.Start(ctx, client.StartOptions{})
		if err != nil {
			return fmt.Errorf("start workspace %w", err)
		}
		return nil
	}
	return fmt.Errorf("workspace is stopped")
}

func handleNotFoundStatus(ctx context.Context, workspaceClient client.WorkspaceClient, create bool) error {
	if create {
		err := workspaceClient.Create(ctx, client.CreateOptions{})
		if err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("workspace not found")
}

// BuildAgentClientOptions contains parameters for BuildAgentClient
type BuildAgentClientOptions struct {
	WorkspaceClient client.WorkspaceClient
	CLIOptions      provider.CLIOptions
	AgentCommand    string
	Log             log.Logger
	TunnelOptions   []tunnelserver.Option
}

// BuildAgentClient builds an agent client for workspace operations
func BuildAgentClient(ctx context.Context, opts BuildAgentClientOptions) (*config2.Result, error) {
	workspaceInfo, wInfo, err := opts.WorkspaceClient.AgentInfo(opts.CLIOptions)
	if err != nil {
		return nil, err
	}

	command := buildAgentCommand(opts.WorkspaceClient, opts.AgentCommand, workspaceInfo, opts.Log)
	stdoutReader, stdoutWriter, stdinReader, stdinWriter, err := createPipes()
	if err != nil {
		return nil, err
	}
	defer func() { _ = stdoutWriter.Close() }()
	defer func() { _ = stdoutReader.Close() }()
	defer func() { _ = stdinReader.Close() }()
	defer func() { _ = stdinWriter.Close() }()

	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errChan := runAgentInjection(agentInjectionOptions{
		ctx:             cancelCtx,
		workspaceClient: opts.WorkspaceClient,
		command:         command,
		stdin:           stdinReader,
		stdout:          stdoutWriter,
		timeout:         wInfo.InjectTimeout,
		log:             opts.Log,
		cancel:          cancel,
	})
	result, err := runTunnelServer(cancelCtx, opts, stdoutReader, stdinWriter)
	if err != nil {
		return nil, err
	}

	return result, <-errChan
}

func buildAgentCommand(workspaceClient client.WorkspaceClient, agentCommand, workspaceInfo string, log log.Logger) string {
	command := fmt.Sprintf("'%s' agent workspace %s --workspace-info '%s'", workspaceClient.AgentPath(), agentCommand, workspaceInfo)
	if log.GetLevel() == logrus.DebugLevel {
		command += " --debug"
	}
	return command
}

func createPipes() (stdoutReader, stdoutWriter, stdinReader, stdinWriter *os.File, err error) {
	stdoutReader, stdoutWriter, err = os.Pipe()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	stdinReader, stdinWriter, err = os.Pipe()
	if err != nil {
		func() { _ = stdoutReader.Close() }()
		func() { _ = stdoutWriter.Close() }()
		return nil, nil, nil, nil, err
	}
	return stdoutReader, stdoutWriter, stdinReader, stdinWriter, nil
}

type agentInjectionOptions struct {
	ctx             context.Context
	workspaceClient client.WorkspaceClient
	command         string
	stdin           *os.File
	stdout          *os.File
	timeout         time.Duration
	log             log.Logger
	cancel          context.CancelFunc
}

func runAgentInjection(opts agentInjectionOptions) chan error {
	errChan := make(chan error, 1)
	go func() {
		defer opts.log.Debugf("up command completed")
		defer opts.cancel()

		writer := opts.log.ErrorStreamOnly().Writer(logrus.InfoLevel, false)
		defer func() { _ = writer.Close() }()

		errChan <- agent.InjectAgent(&agent.InjectOptions{
			Ctx: opts.ctx,
			Exec: func(ctx context.Context, command string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
				return opts.workspaceClient.Command(ctx, client.CommandOptions{
					Command: command,
					Stdin:   stdin,
					Stdout:  stdout,
					Stderr:  stderr,
				})
			},
			IsLocal:         opts.workspaceClient.AgentLocal(),
			RemoteAgentPath: opts.workspaceClient.AgentPath(),
			DownloadURL:     opts.workspaceClient.AgentURL(),
			Command:         opts.command,
			Stdin:           opts.stdin,
			Stdout:          opts.stdout,
			Stderr:          writer,
			Log:             opts.log.ErrorStreamOnly(),
			Timeout:         opts.timeout,
		})
	}()
	return errChan
}

func runTunnelServer(ctx context.Context, opts BuildAgentClientOptions, stdoutReader, stdinWriter *os.File) (*config2.Result, error) {
	result, err := tunnelserver.RunUpServer(
		ctx,
		stdoutReader,
		stdinWriter,
		opts.WorkspaceClient.AgentInjectGitCredentials(opts.CLIOptions),
		opts.WorkspaceClient.AgentInjectDockerCredentials(opts.CLIOptions),
		opts.WorkspaceClient.WorkspaceConfig(),
		opts.Log,
		opts.TunnelOptions...,
	)
	if err != nil {
		return nil, fmt.Errorf("run tunnel server: %w", err)
	}
	return result, nil
}
