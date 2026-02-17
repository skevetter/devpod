package clientimplementation

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/pkg/agent"
	"github.com/skevetter/devpod/pkg/agent/tunnelserver"
	"github.com/skevetter/devpod/pkg/client"
	config2 "github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/skevetter/devpod/pkg/provider"
	"github.com/skevetter/devpod/pkg/shell"
	"github.com/skevetter/devpod/pkg/ssh"
	"github.com/skevetter/devpod/pkg/types"
	"github.com/skevetter/log"
)

func RunCommandWithBinaries(opts CommandOptions) error {
	environ, err := provider.ToEnvironmentWithBinaries(provider.EnvironmentOptions{
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
			return fmt.Errorf("start workspace: %w", err)
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
