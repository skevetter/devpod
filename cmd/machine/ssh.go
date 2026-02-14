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
	termModeUsage    = "PTY TERM selection mode: auto, strict, fallback"
	installUsage     = "Install local TERM terminfo on remote before PTY"
)

type SSHSessionOptions struct {
	TermMode        string
	InstallTerminfo bool
}

type StartSSHSessionOptions struct {
	User            string
	Command         string
	AgentForwarding bool
	SessionOptions  SSHSessionOptions
	Exec            ExecFunc
	Stderr          io.Writer
}

type RunSSHSessionOptions struct {
	Command         string
	AgentForwarding bool
	SessionOptions  SSHSessionOptions
	Stderr          io.Writer
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
	sshCmd.Flags().StringVar(&cmd.TermMode, "term-mode", TermModeAuto, termModeUsage)
	sshCmd.Flags().BoolVar(&cmd.InstallTerminfo, "install-terminfo", false, installUsage)
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
	return StartSSHSession(ctx, StartSSHSessionOptions{
		Command:         cmd.Command,
		AgentForwarding: cmd.AgentForwarding,
		SessionOptions:  SSHSessionOptions{TermMode: cmd.TermMode, InstallTerminfo: cmd.InstallTerminfo},
		Exec: func(ctx context.Context, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
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
		},
		Stderr: writer,
	})
}

type ExecFunc func(ctx context.Context, stdin io.Reader, stdout io.Writer, stderr io.Writer) error

func StartSSHSession(ctx context.Context, options StartSSHSessionOptions) error {
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
		errChan <- options.Exec(ctx, stdinReader, stdoutWriter, options.Stderr)
	}()

	sshClient, err := devssh.StdioClientWithUser(stdoutReader, stdinWriter, options.User, false)
	if err != nil {
		return err
	}
	defer func() { _ = sshClient.Close() }()

	return RunSSHSession(ctx, sshClient, RunSSHSessionOptions{
		Command:         options.Command,
		AgentForwarding: options.AgentForwarding,
		SessionOptions:  options.SessionOptions,
		Stderr:          options.Stderr,
	})
}

func RunSSHSession(ctx context.Context, sshClient *ssh.Client, options RunSSHSessionOptions) error {
	// create a new session
	session, err := sshClient.NewSession()
	if err != nil {
		return err
	}
	defer func() { _ = session.Close() }()

	// request agent forwarding
	authSock := devsshagent.GetSSHAuthSocket()
	if options.AgentForwarding && authSock != "" {
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

		t, err := resolvePTYTerm(ctx, sshClient, options.SessionOptions)
		if err != nil {
			t = defaultTerm
			_, _ = fmt.Fprintf(
				options.Stderr,
				"warning: failed to resolve TERM, falling back to %s: %v\n",
				defaultTerm,
				err,
			)
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
	session.Stderr = options.Stderr
	if options.Command == "" {
		if err := session.Shell(); err != nil {
			return fmt.Errorf("start ssh session with shell: %w", err)
		}
	} else {
		if err := session.Start(options.Command); err != nil {
			return fmt.Errorf("start ssh session with command %s: %w", options.Command, err)
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

	if mode == TermModeStrict {
		return localTerm, nil
	}
	if mode == TermModeFallback {
		return defaultTerm, nil
	}
	if mode != TermModeAuto {
		return "", fmt.Errorf("invalid --term-mode %q: expected one of auto, strict, fallback", mode)
	}

	return resolveAutoPTYTerm(ctx, sshClient, localTerm, sessionOptions.InstallTerminfo)
}

func resolveAutoPTYTerm(ctx context.Context, sshClient *ssh.Client, localTerm string, installTerminfo bool) (string, error) {
	supported, err := remoteTerminfoExists(sshClient, localTerm)
	if err != nil {
		return defaultTerm, nil
	}
	if supported {
		return localTerm, nil
	}
	if !installTerminfo {
		return defaultTerm, nil
	}

	installed, err := installLocalTerminfoOnRemote(ctx, sshClient, localTerm)
	if err != nil {
		return defaultTerm, nil
	}
	if !installed {
		return defaultTerm, nil
	}

	supported, err = remoteTerminfoExists(sshClient, localTerm)
	if err != nil {
		return defaultTerm, nil
	}
	if supported {
		return localTerm, nil
	}

	return defaultTerm, nil
}

func remoteTerminfoExists(sshClient *ssh.Client, term string) (bool, error) {
	check := fmt.Sprintf(
		"command -v infocmp >/dev/null 2>&1 && TERM=%s infocmp -x %s >/dev/null 2>&1",
		shellQuote(term),
		shellQuote(term),
	)
	command := fmt.Sprintf("sh -lc %s", shellQuote(check))
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
	if len(output) == 0 {
		return false, nil
	}

	command := "sh -lc 'command -v tic >/dev/null 2>&1 && tic -x - >/dev/null 2>&1'"
	session, err := sshClient.NewSession()
	if err != nil {
		return false, fmt.Errorf("create ssh session for terminfo install: %w", err)
	}
	defer func() { _ = session.Close() }()

	stdin, err := session.StdinPipe()
	if err != nil {
		return false, fmt.Errorf("get remote stdin for terminfo install: %w", err)
	}

	errChan := streamBytesToWriter(bytes.NewReader(output), stdin)

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

func streamBytesToWriter(reader io.Reader, writer io.WriteCloser) <-chan error {
	errChan := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(writer, reader)
		closeErr := writer.Close()
		if copyErr != nil {
			errChan <- copyErr
			return
		}
		errChan <- closeErr
	}()

	return errChan
}

func runRemoteCommandExitCode(sshClient *ssh.Client, command string) (int, error) {
	session, err := sshClient.NewSession()
	if err != nil {
		return -1, fmt.Errorf("create ssh session: %w", err)
	}
	defer func() { _ = session.Close() }()

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
