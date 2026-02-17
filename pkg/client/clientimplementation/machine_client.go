package clientimplementation

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/pkg/client"
	"github.com/skevetter/devpod/pkg/config"
	"github.com/skevetter/devpod/pkg/options"
	"github.com/skevetter/devpod/pkg/provider"
	"github.com/skevetter/devpod/pkg/types"
	"github.com/skevetter/log"
)

func NewMachineClient(devPodConfig *config.Config, provider *provider.ProviderConfig, machine *provider.Machine, log log.Logger) (client.MachineClient, error) {
	if !provider.IsMachineProvider() {
		log.Error("provider is not a machine provider")
		return nil, fmt.Errorf("Provider is not a machine provider. Use another provider")
	} else if machine == nil {
		return nil, fmt.Errorf("Machine does not exist. Perhaps it was deleted without the workspace being deleted")
	}

	mc := &machineClient{
		devPodConfig: devPodConfig,
		config:       provider,
		machine:      machine,
		log:          log,
	}
	mc.executor = &machineExecutor{client: mc}

	return mc, nil
}

type machineClient struct {
	devPodConfig *config.Config
	config       *provider.ProviderConfig
	machine      *provider.Machine
	log          log.Logger
	executor     *machineExecutor
}

// machineExecutor handles command execution with common patterns.
type machineExecutor struct {
	client *machineClient
}

// execConfig defines how to execute a machine command.
type execConfig struct {
	name         string
	command      types.StrArray
	stdout       io.Writer
	stderr       io.Writer
	extraEnv     map[string]string
	stdin        io.Reader
	log          log.Logger
	withProgress bool
	startMsg     string
	doneMsg      string
}

func (e *machineExecutor) execute(ctx context.Context, cfg execConfig) error {
	var done chan struct{}
	if cfg.withProgress {
		done = scheduleLogMessage("Devpod "+cfg.name+" operation is in progress", e.client.log)
		defer close(done)
	}

	if cfg.startMsg != "" {
		e.client.log.Infof(cfg.startMsg)
	}

	opts := CommandOptions{
		Ctx:      ctx,
		Name:     cfg.name,
		Command:  cfg.command,
		Context:  e.client.machine.Context,
		Machine:  e.client.machine,
		Options:  e.client.devPodConfig.ProviderOptions(e.client.config.Name),
		Config:   e.client.config,
		Stdout:   cfg.stdout,
		Stderr:   cfg.stderr,
		ExtraEnv: cfg.extraEnv,
		Stdin:    cfg.stdin,
		Log:      cfg.log,
	}

	if opts.Log == nil {
		opts.Log = e.client.log
	}

	err := RunCommandWithBinaries(opts)
	if err != nil {
		return err
	}

	if cfg.doneMsg != "" {
		e.client.log.Done(cfg.doneMsg)
	}

	return nil
}

// lifecycleCommand executes a standard lifecycle operation (create/start/stop).
func (e *machineExecutor) lifecycleCommand(ctx context.Context, name string, command types.StrArray, verb, pastVerb string) error {
	writer := e.client.log.Writer(logrus.InfoLevel, false)
	defer func() { _ = writer.Close() }()

	return e.execute(ctx, execConfig{
		name:         name,
		command:      command,
		stdout:       writer,
		stderr:       writer,
		withProgress: true,
		startMsg:     verb + " machine",
		doneMsg:      pastVerb + " machine",
	})
}

func (s *machineClient) Provider() string {
	return s.config.Name
}

func (s *machineClient) Machine() string {
	return s.machine.ID
}

func (s *machineClient) MachineConfig() *provider.Machine {
	return provider.CloneMachine(s.machine)
}

func (s *machineClient) RefreshOptions(ctx context.Context, userOptionsRaw []string, reconfigure bool) error {
	userOptions, err := provider.ParseOptions(userOptionsRaw)
	if err != nil {
		return fmt.Errorf("parse options: %w", err)
	}

	machine, err := options.ResolveAndSaveOptionsMachine(ctx, s.devPodConfig, s.config, s.machine, userOptions, s.log)
	if err != nil {
		return err
	}

	s.machine = machine
	return nil
}

func (s *machineClient) AgentPath() string {
	return options.ResolveAgentConfig(s.devPodConfig, s.config, nil, s.machine).Path
}

func (s *machineClient) AgentLocal() bool {
	return options.ResolveAgentConfig(s.devPodConfig, s.config, nil, s.machine).Local == "true"
}

func (s *machineClient) AgentURL() string {
	return options.ResolveAgentConfig(s.devPodConfig, s.config, nil, s.machine).DownloadURL
}

func (s *machineClient) Context() string {
	return s.machine.Context
}

func (s *machineClient) Create(ctx context.Context, options client.CreateOptions) error {
	return s.executor.lifecycleCommand(ctx, "create", s.config.Exec.Create, "creating", "created")
}

func (s *machineClient) Start(ctx context.Context, options client.StartOptions) error {
	return s.executor.lifecycleCommand(ctx, "start", s.config.Exec.Start, "starting", "started")
}

func (s *machineClient) Stop(ctx context.Context, options client.StopOptions) error {
	return s.executor.lifecycleCommand(ctx, "stop", s.config.Exec.Stop, "stopping", "stopped")
}

func (s *machineClient) Command(ctx context.Context, commandOptions client.CommandOptions) error {
	return s.executor.execute(ctx, execConfig{
		name:    "command",
		command: s.config.Exec.Command,
		stdout:  commandOptions.Stdout,
		stderr:  commandOptions.Stderr,
		stdin:   commandOptions.Stdin,
		extraEnv: map[string]string{
			provider.CommandEnv: commandOptions.Command,
		},
		log: s.log.ErrorStreamOnly(),
	})
}

func (s *machineClient) Status(ctx context.Context, options client.StatusOptions) (client.Status, error) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	err := s.executor.execute(ctx, execConfig{
		name:    "status",
		command: s.config.Exec.Status,
		stdout:  stdout,
		stderr:  io.MultiWriter(stderr, s.log.Writer(logrus.InfoLevel, true)),
	})
	if err != nil {
		return client.StatusNotFound, fmt.Errorf("get status: %s%s", strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()))
	}

	parsedStatus, err := client.ParseStatus(stdout.String())
	if err != nil {
		return client.StatusNotFound, err
	}

	return parsedStatus, nil
}

func (s *machineClient) Delete(ctx context.Context, options client.DeleteOptions) error {
	if options.GracePeriod != "" {
		if duration, err := time.ParseDuration(options.GracePeriod); err == nil {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, duration)
			defer cancel()
		}
	}

	writer := s.log.Writer(logrus.InfoLevel, false)
	defer func() { _ = writer.Close() }()

	err := s.executor.execute(ctx, execConfig{
		name:         "delete",
		command:      s.config.Exec.Delete,
		stdout:       writer,
		stderr:       writer,
		withProgress: true,
		startMsg:     "deleting machine",
		doneMsg:      "deleted machine",
	})

	if err != nil && !options.Force {
		return err
	}
	if err != nil {
		s.log.WithFields(logrus.Fields{"machineId": s.machine.ID, "err": err}).Errorf("failed to delete machine")
	}

	return DeleteMachineFolder(s.machine.Context, s.machine.ID)
}

func scheduleLogMessage(msg string, log log.Logger) chan struct{} {
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			case <-time.After(time.Second * 5):
				log.Info(msg)
			}
		}
	}()

	return done
}
