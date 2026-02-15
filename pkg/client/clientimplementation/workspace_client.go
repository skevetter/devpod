package clientimplementation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gofrs/flock"
	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/pkg/client"
	"github.com/skevetter/devpod/pkg/config"
	"github.com/skevetter/devpod/pkg/options"
	"github.com/skevetter/devpod/pkg/provider"
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

func (s *workspaceClient) Context() string {
	return s.workspace.Context
}

func (s *workspaceClient) RefreshOptions(ctx context.Context, userOptionsRaw []string, reconfigure bool) error {
	s.m.Lock()
	defer s.m.Unlock()

	userOptions, err := provider.ParseOptions(userOptionsRaw)
	if err != nil {
		return fmt.Errorf("parse options: %w", err)
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

func (s *workspaceClient) initLock() {
	s.workspaceLockOnce.Do(func() {
		s.m.Lock()
		defer s.m.Unlock()

		// get locks dir
		workspaceLocksDir, err := provider.GetLocksDir(s.workspace.Context)
		if err != nil {
			panic(fmt.Errorf("get workspaces dir: %w", err))
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
		return fmt.Errorf("error locking workspace: %w", err)
	}
	s.log.Debug("acquired workspace lock")

	// try to lock machine
	if s.machineLock != nil {
		s.log.Debug("acquire machine lock")
		err := tryLock(ctx, s.machineLock, "machine", s.log)
		if err != nil {
			return fmt.Errorf("error locking machine: %w", err)
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
		return false, fmt.Errorf("retrieve machine status: %w", err)
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
	environ, err := s.buildEnvironment(commandOptions.Command)
	if err != nil {
		return err
	}

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

func (s *workspaceClient) buildEnvironment(command string) ([]string, error) {
	s.m.Lock()
	defer s.m.Unlock()

	return provider.ToEnvironmentWithBinaries(provider.EnvironmentOptions{
		Context:   s.workspace.Context,
		Workspace: s.workspace,
		Machine:   s.machine,
		Options:   s.devPodConfig.ProviderOptions(s.config.Name),
		Config:    s.config,
		ExtraEnv: map[string]string{
			provider.CommandEnv: command,
		},
		Log: s.log,
	})
}
