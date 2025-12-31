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
		log.WithFields(logrus.Fields{
			"provider": provider.Name,
		}).Error("provider is not a machine provider")
		return nil, fmt.Errorf("Provider is not a machine provider. Use another provider")
	} else if machine == nil {
		return nil, fmt.Errorf("Machine does not exist. Perhaps it was deleted without the workspace being deleted")
	}

	return &machineClient{
		devPodConfig: devPodConfig,
		config:       provider,
		machine:      machine,
		log:          log,
	}, nil
}

type machineClient struct {
	devPodConfig *config.Config
	config       *provider.ProviderConfig
	machine      *provider.Machine
	log          log.Logger
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
		return fmt.Errorf("parse options %w", err)
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
	done := printStillRunningLogMessagePeriodically(s.log)
	defer close(done)

	writer := s.log.Writer(logrus.InfoLevel, false)
	defer func() { _ = writer.Close() }()

	// create a machine
	s.log.WithFields(logrus.Fields{
		"machineId": s.machine.ID,
		"provider":  s.config.Name,
	}).Infof("creating machine")
	err := RunCommandWithBinaries(
		ctx,
		"create",
		s.config.Exec.Create,
		s.machine.Context,
		nil,
		s.machine,
		s.devPodConfig.ProviderOptions(s.config.Name),
		s.config,
		nil,
		nil,
		writer,
		writer,
		s.log,
	)
	if err != nil {
		return err
	}

	s.log.WithFields(logrus.Fields{
		"machineId": s.machine.ID,
		"provider":  s.config.Name,
	}).Done("created machine")
	return nil
}

func (s *machineClient) Start(ctx context.Context, options client.StartOptions) error {
	done := printStillRunningLogMessagePeriodically(s.log)
	defer close(done)

	writer := s.log.Writer(logrus.InfoLevel, false)
	defer func() { _ = writer.Close() }()

	s.log.WithFields(logrus.Fields{
		"machineId": s.machine.ID,
	}).Infof("starting machine")
	err := RunCommandWithBinaries(
		ctx,
		"start",
		s.config.Exec.Start,
		s.machine.Context,
		nil,
		s.machine,
		s.devPodConfig.ProviderOptions(s.config.Name),
		s.config,
		nil,
		nil,
		writer,
		writer,
		s.log,
	)
	if err != nil {
		return err
	}
	s.log.WithFields(logrus.Fields{
		"machineId": s.machine.ID,
	}).Done("started machine")

	return nil
}

func (s *machineClient) Stop(ctx context.Context, options client.StopOptions) error {
	done := printStillRunningLogMessagePeriodically(s.log)
	defer close(done)

	writer := s.log.Writer(logrus.InfoLevel, false)
	defer func() { _ = writer.Close() }()

	s.log.WithFields(logrus.Fields{
		"machineId": s.machine.ID,
	}).Infof("stopping machine")
	err := RunCommandWithBinaries(
		ctx,
		"stop",
		s.config.Exec.Stop,
		s.machine.Context,
		nil,
		s.machine,
		s.devPodConfig.ProviderOptions(s.config.Name),
		s.config,
		nil,
		nil,
		writer,
		writer,
		s.log,
	)
	if err != nil {
		return err
	}
	s.log.WithFields(logrus.Fields{
		"machineId": s.machine.ID,
	}).Donef("stopped machine")

	return nil
}

func (s *machineClient) Command(ctx context.Context, commandOptions client.CommandOptions) error {
	return RunCommandWithBinaries(
		ctx,
		"command",
		s.config.Exec.Command,
		s.machine.Context,
		nil,
		s.machine,
		s.devPodConfig.ProviderOptions(s.config.Name),
		s.config,
		map[string]string{
			provider.CommandEnv: commandOptions.Command,
		},
		commandOptions.Stdin,
		commandOptions.Stdout,
		commandOptions.Stderr,
		s.log.ErrorStreamOnly(),
	)
}

func (s *machineClient) Status(ctx context.Context, options client.StatusOptions) (client.Status, error) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := RunCommandWithBinaries(
		ctx,
		"status",
		s.config.Exec.Status,
		s.machine.Context,
		nil,
		s.machine,
		s.devPodConfig.ProviderOptions(s.config.Name),
		s.config,
		nil,
		nil,
		stdout,
		stderr,
		s.log,
	)
	if err != nil {
		return client.StatusNotFound, fmt.Errorf("get status: %s%s", strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()))
	}

	// parse status
	parsedStatus, err := client.ParseStatus(stdout.String())
	if err != nil {
		return client.StatusNotFound, err
	}

	return parsedStatus, nil
}

func (s *machineClient) Delete(ctx context.Context, options client.DeleteOptions) error {
	var gracePeriod *time.Duration
	if options.GracePeriod != "" {
		duration, err := time.ParseDuration(options.GracePeriod)
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

	done := printStillRunningLogMessagePeriodically(s.log)
	defer close(done)

	writer := s.log.Writer(logrus.InfoLevel, false)
	defer func() { _ = writer.Close() }()

	s.log.WithFields(logrus.Fields{
		"machineId": s.machine.ID,
	}).Infof("deleting machine")
	err := RunCommandWithBinaries(
		ctx,
		"delete",
		s.config.Exec.Delete,
		s.machine.Context,
		nil,
		s.machine,
		s.devPodConfig.ProviderOptions(s.config.Name),
		s.config,
		nil,
		nil,
		writer,
		writer,
		s.log,
	)
	if err != nil {
		if !options.Force {
			return err
		}

		s.log.WithFields(logrus.Fields{
			"machineId": s.machine.ID,
			"err":       err,
		}).Errorf("failed to delete machine")
	}
	s.log.WithFields(logrus.Fields{
		"machineId": s.machine.ID,
	}).Done("deleted machine")

	// delete machine folder
	err = DeleteMachineFolder(s.machine.Context, s.machine.ID)
	if err != nil {
		return err
	}

	return nil
}

func runCommand(ctx context.Context, name string, command types.StrArray, environ []string, stdin io.Reader, stdout io.Writer, stderr io.Writer, log log.Logger) (err error) {
	if len(command) == 0 {
		return nil
	}

	// log
	log.WithFields(logrus.Fields{
		"name":    name,
		"command": strings.Join(command, " "),
	}).Debug("run provider command")

	// set debug level
	if log.GetLevel() == logrus.DebugLevel {
		environ = append(environ, DevPodDebug+"=true")
	}

	// run the command
	return RunCommand(ctx, command, environ, stdin, stdout, stderr)
}

func printStillRunningLogMessagePeriodically(log log.Logger) chan struct{} {
	return printLogMessagePeriodically("Please wait. DevPod is still running, this might take a while", log)
}

func printLogMessagePeriodically(message string, log log.Logger) chan struct{} {
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			case <-time.After(time.Second * 5):
				log.Info(message)
			}
		}
	}()

	return done
}
