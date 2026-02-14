package machine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/cmd/flags"
	devagent "github.com/skevetter/devpod/pkg/agent"
	"github.com/skevetter/devpod/pkg/client"
	"github.com/skevetter/devpod/pkg/config"
	devssh "github.com/skevetter/devpod/pkg/ssh"
	devsshagent "github.com/skevetter/devpod/pkg/ssh/agent"
	"github.com/skevetter/devpod/pkg/workspace"
	"github.com/skevetter/log"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

// SSHCmd holds the configuration
type SSHCmd struct {
	*flags.GlobalFlags

	Command         string
	AgentForwarding bool
	TermMode        string
	InstallTerminfo bool
}

const (
	TermModeAuto     = "auto"
	TermModeStrict   = "strict"
	TermModeFallback = "fallback"
	defaultTerm      = "xterm-256color"
)

type SSHSessionOptions struct {
	TermMode        string
	InstallTerminfo bool
}

// NewSSHCmd creates a new destroy command
func NewSSHCmd(flags *flags.GlobalFlags) *cobra.Command {
	cmd := &SSHCmd{
		GlobalFlags: flags,
	}
	sshCmd := &cobra.Command{
		Use:   "ssh [name]",
		Short: "SSH into the machine",
		RunE: func(c *cobra.Command, args []string) error {
			return cmd.Run(context.Background(), args)
		},
	}

	sshCmd.Flags().StringVar(&cmd.Command, "command", "", "The command to execute on the remote machine")
	sshCmd.Flags().BoolVar(&cmd.AgentForwarding, "agent-forwarding", false, "If true, will forward the local ssh keys")
	sshCmd.Flags().StringVar(&cmd.TermMode, "term-mode", TermModeAuto, "PTY TERM selection mode: auto, strict, fallback")
	sshCmd.Flags().BoolVar(&cmd.InstallTerminfo, "install-terminfo", false, "Install local TERM terminfo on the remote before opening a PTY")
	return sshCmd
}

// Run runs the command logic
func (cmd *SSHCmd) Run(ctx context.Context, args []string) error {
	devPodConfig, err := config.LoadConfig(cmd.Context, cmd.Provider)
	if err != nil {
		return err
	}

	machineClient, err := workspace.GetMachine(devPodConfig, args, log.Default)
	if err != nil {
		return err
	}

	writer := log.Default.ErrorStreamOnly().Writer(logrus.InfoLevel, false)
	defer func() { _ = writer.Close() }()

	// Get the timeout from the context options
	timeout := config.ParseTimeOption(devPodConfig, config.ContextOptionAgentInjectTimeout)

	// start the ssh session
	return StartSSHSession(
		ctx,
		"",
		cmd.Command,
		cmd.AgentForwarding,
		SSHSessionOptions{TermMode: cmd.TermMode, InstallTerminfo: cmd.InstallTerminfo},
		func(ctx context.Context, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
			command := fmt.Sprintf("'%s' helper ssh-server --stdio", machineClient.AgentPath())
			if cmd.Debug {
				command += " --debug"
			}
			return devagent.InjectAgent(&devagent.InjectOptions{
				Ctx: ctx,
				Exec: func(ctx context.Context, command string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
					return machineClient.Command(ctx, client.CommandOptions{
						Command: command,
						Stdin:   stdin,
						Stdout:  stdout,
						Stderr:  stderr,
					})
				},
				IsLocal:         machineClient.AgentLocal(),
				RemoteAgentPath: machineClient.AgentPath(),
				DownloadURL:     machineClient.AgentURL(),
				Command:         command,
				Stdin:           stdin,
				Stdout:          stdout,
				Stderr:          stderr,
				Log:             log.Default.ErrorStreamOnly(),
				Timeout:         timeout,
			})
		}, writer)
}

type ExecFunc func(ctx context.Context, stdin io.Reader, stdout io.Writer, stderr io.Writer) error

func StartSSHSession(ctx context.Context, user, command string, agentForwarding bool, sessionOptions SSHSessionOptions, exec ExecFunc, stderr io.Writer) error {
	// create readers
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		return err
	}
	defer func() { _ = stdoutReader.Close() }()
	defer func() { _ = stdoutWriter.Close() }()
	stdinReader, stdinWriter, err := os.Pipe()
	if err != nil {
		return err
	}
	defer func() { _ = stdinWriter.Close() }()
	defer func() { _ = stdinReader.Close() }()

	// start ssh machine
	errChan := make(chan error, 1)
	go func() {
		errChan <- exec(ctx, stdinReader, stdoutWriter, stderr)
	}()

	sshClient, err := devssh.StdioClientWithUser(stdoutReader, stdinWriter, user, false)
	if err != nil {
		return err
	}
	defer func() { _ = sshClient.Close() }()

	return RunSSHSession(ctx, sshClient, agentForwarding, command, sessionOptions, stderr)
}

func RunSSHSession(ctx context.Context, sshClient *ssh.Client, agentForwarding bool, command string, sessionOptions SSHSessionOptions, stderr io.Writer) error {
	// create a new session
	session, err := sshClient.NewSession()
	if err != nil {
		return err
	}
	defer func() { _ = session.Close() }()

	// request agent forwarding
	authSock := devsshagent.GetSSHAuthSocket()
	if agentForwarding && authSock != "" {
		err = devsshagent.ForwardToRemote(sshClient, authSock)
		if err != nil {
			return fmt.Errorf("forward agent: %w", err)
		}

		err = devsshagent.RequestAgentForwarding(session)
		if err != nil {
			return fmt.Errorf("request agent forwarding: %w", err)
		}
	}

	stdout := os.Stdout
	stdin := os.Stdin

	if isatty.IsTerminal(stdout.Fd()) {
		state, err := term.MakeRaw(int(stdout.Fd()))
		if err != nil {
			return err
		}
		defer func() {
			_ = term.Restore(int(stdout.Fd()), state)
		}()

		windowChange := devssh.WatchWindowSize(ctx)
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case <-windowChange:
				}
				width, height, err := term.GetSize(int(stdout.Fd()))
				if err != nil {
					continue
				}
				_ = session.WindowChange(height, width)
			}
		}()

		t, err := resolvePTYTerm(ctx, sshClient, sessionOptions)
		if err != nil {
			return err
		}
		// get initial window size
		width, height := 80, 40
		if w, h, err := term.GetSize(int(stdout.Fd())); err == nil {
			width, height = w, h
		}
		if err = session.RequestPty(t, height, width, ssh.TerminalModes{}); err != nil {
			return fmt.Errorf("request pty: %w", err)
		}
	}

	session.Stdin = stdin
	session.Stdout = stdout
	session.Stderr = stderr
	if command == "" {
		if err := session.Shell(); err != nil {
			return fmt.Errorf("start ssh session with shell: %w", err)
		}
	} else {
		if err := session.Start(command); err != nil {
			return fmt.Errorf("start ssh session with command %s: %w", command, err)
		}
	}

	if err := session.Wait(); err != nil {
		return fmt.Errorf("ssh session: %w", err)
	}

	return nil
}

func resolvePTYTerm(ctx context.Context, sshClient *ssh.Client, sessionOptions SSHSessionOptions) (string, error) {
	localTerm := defaultTerm
	if termEnv, ok := os.LookupEnv("TERM"); ok && termEnv != "" {
		localTerm = termEnv
	}

	mode := sessionOptions.TermMode
	if mode == "" {
		mode = TermModeAuto
	}

	switch mode {
	case TermModeStrict:
		return localTerm, nil
	case TermModeFallback:
		return defaultTerm, nil
	case TermModeAuto:
		supported, err := remoteTerminfoExists(sshClient, localTerm)
		if err != nil {
			return "", err
		}
		if supported {
			return localTerm, nil
		}

		if sessionOptions.InstallTerminfo {
			installed, err := installLocalTerminfoOnRemote(ctx, sshClient, localTerm)
			if err != nil {
				return "", err
			}
			if installed {
				supported, err = remoteTerminfoExists(sshClient, localTerm)
				if err != nil {
					return "", err
				}
				if supported {
					return localTerm, nil
				}
			}
		}

		return defaultTerm, nil
	default:
		return "", fmt.Errorf("invalid --term-mode %q: expected one of auto, strict, fallback", mode)
	}
}

func remoteTerminfoExists(sshClient *ssh.Client, term string) (bool, error) {
	command := fmt.Sprintf("sh -lc %s", shellQuote(fmt.Sprintf("command -v infocmp >/dev/null 2>&1 && TERM=%s infocmp -x \"$TERM\" >/dev/null 2>&1", shellQuote(term))))
	code, err := runRemoteCommandExitCode(sshClient, command)
	if err != nil {
		return false, err
	}

	if code == 0 {
		return true, nil
	}

	if code == 1 || code == 127 {
		return false, nil
	}

	return false, nil
}

func installLocalTerminfoOnRemote(ctx context.Context, sshClient *ssh.Client, term string) (bool, error) {
	infocmpCmd := exec.CommandContext(ctx, "infocmp", "-x", term)
	output, err := infocmpCmd.Output()
	if err != nil {
		return false, nil
	}

	command := "sh -lc 'command -v tic >/dev/null 2>&1 && tic -x - >/dev/null 2>&1'"
	session, err := sshClient.NewSession()
	if err != nil {
		return false, fmt.Errorf("create ssh session for terminfo install: %w", err)
	}
	defer session.Close()

	stdin, err := session.StdinPipe()
	if err != nil {
		return false, fmt.Errorf("get remote stdin for terminfo install: %w", err)
	}

	errChan := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(stdin, bytes.NewReader(output))
		closeErr := stdin.Close()
		if copyErr != nil {
			errChan <- copyErr
			return
		}
		errChan <- closeErr
	}()

	runErr := session.Run(command)
	copyErr := <-errChan
	if copyErr != nil {
		return false, nil
	}
	if runErr != nil {
		var exitErr *ssh.ExitError
		if errors.As(runErr, &exitErr) {
			return false, nil
		}
		return false, fmt.Errorf("run remote tic command: %w", runErr)
	}

	return true, nil
}

func runRemoteCommandExitCode(sshClient *ssh.Client, command string) (int, error) {
	session, err := sshClient.NewSession()
	if err != nil {
		return -1, fmt.Errorf("create ssh session: %w", err)
	}
	defer session.Close()

	err = session.Run(command)
	if err == nil {
		return 0, nil
	}

	var exitErr *ssh.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitStatus(), nil
	}

	return -1, fmt.Errorf("run remote command: %w", err)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
