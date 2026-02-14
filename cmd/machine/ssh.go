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
	_ = sshCmd.RegisterFlagCompletionFunc(
		"term-mode",
		func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			return []string{TermModeAuto, TermModeStrict, TermModeFallback}, cobra.ShellCompDirectiveNoFileComp
		},
	)
	return sshCmd
}

// Run runs the command logic
func (cmd *SSHCmd) Run(ctx context.Context, args []string) error {
	if !isValidTermMode(cmd.TermMode) {
		return fmt.Errorf("invalid --term-mode %q: expected one of auto, strict, fallback", cmd.TermMode)
	}

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
	if options.Stderr == nil {
		options.Stderr = os.Stderr
	}

	session, err := sshClient.NewSession()
	if err != nil {
		return err
	}
	defer func() { _ = session.Close() }()

	if err := configureAgentForwarding(sshClient, session, options.AgentForwarding); err != nil {
		return err
	}

	if err := setupInteractivePTY(ctx, sshClient, session, options); err != nil {
		return err
	}

	wireSessionStdio(session, options.Stderr)
	if err := startSessionCommand(session, options.Command); err != nil {
		return err
	}

	if err := session.Wait(); err != nil {
		return fmt.Errorf("ssh session: %w", err)
	}

	return nil
}

func configureAgentForwarding(sshClient *ssh.Client, session *ssh.Session, shouldForward bool) error {
	authSock := devsshagent.GetSSHAuthSocket()
	if !shouldForward || authSock == "" {
		return nil
	}

	err := devsshagent.ForwardToRemote(sshClient, authSock)
	if err != nil {
		return fmt.Errorf("forward agent: %w", err)
	}

	err = devsshagent.RequestAgentForwarding(session)
	if err != nil {
		return fmt.Errorf("request agent forwarding: %w", err)
	}

	return nil
}

func setupInteractivePTY(
	ctx context.Context,
	sshClient *ssh.Client,
	session *ssh.Session,
	options RunSSHSessionOptions,
) error {
	stdout := os.Stdout
	if !isatty.IsTerminal(stdout.Fd()) {
		return nil
	}

	state, err := term.MakeRaw(int(stdout.Fd()))
	if err != nil {
		return err
	}
	defer func() {
		_ = term.Restore(int(stdout.Fd()), state)
	}()

	startWindowResizeForwarder(ctx, session, int(stdout.Fd()))

	t := resolvePTYTermWithFallback(ctx, sshClient, options.SessionOptions, options.Stderr)
	width, height := getTerminalSize(int(stdout.Fd()))
	if err = session.RequestPty(t, height, width, ssh.TerminalModes{}); err != nil {
		return fmt.Errorf("request pty: %w", err)
	}

	return nil
}

func resolvePTYTermWithFallback(
	ctx context.Context,
	sshClient *ssh.Client,
	sessionOptions SSHSessionOptions,
	stderr io.Writer,
) string {
	t, err := resolvePTYTerm(ctx, sshClient, sessionOptions)
	if err == nil {
		return t
	}

	_, _ = fmt.Fprintf(stderr, "warning: failed to resolve TERM, falling back to %s: %v\n", defaultTerm, err)
	return defaultTerm
}

func startWindowResizeForwarder(ctx context.Context, session *ssh.Session, fd int) {
	windowChange := devssh.WatchWindowSize(ctx)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-windowChange:
			}

			width, height, err := term.GetSize(fd)
			if err != nil {
				continue
			}
			_ = session.WindowChange(height, width)
		}
	}()
}

func getTerminalSize(fd int) (int, int) {
	width, height := 80, 40
	if w, h, err := term.GetSize(fd); err == nil {
		width, height = w, h
	}

	return width, height
}

func wireSessionStdio(session *ssh.Session, stderr io.Writer) {
	session.Stdin = os.Stdin
	session.Stdout = os.Stdout
	session.Stderr = stderr
}

func startSessionCommand(session *ssh.Session, command string) error {
	if command == "" {
		if err := session.Shell(); err != nil {
			return fmt.Errorf("start ssh session with shell: %w", err)
		}
		return nil
	}

	if err := session.Start(command); err != nil {
		return fmt.Errorf("start ssh session with command %s: %w", command, err)
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

func resolveAutoPTYTerm(
	ctx context.Context,
	sshClient *ssh.Client,
	localTerm string,
	installTerminfo bool,
) (string, error) {
	supported, err := remoteTerminfoExists(ctx, sshClient, localTerm)
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

	supported, err = remoteTerminfoExists(ctx, sshClient, localTerm)
	if err != nil {
		return defaultTerm, nil
	}

	return termFromSupport(supported, localTerm), nil
}

func termFromSupport(supported bool, term string) string {
	if supported {
		return term
	}

	return defaultTerm
}

func remoteTerminfoExists(ctx context.Context, sshClient *ssh.Client, term string) (bool, error) {
	check := fmt.Sprintf(
		"command -v infocmp >/dev/null 2>&1 && infocmp -x %s >/dev/null 2>&1",
		shellQuote(term),
	)
	command := fmt.Sprintf("sh -lc %s", shellQuote(check))
	code, err := runRemoteCommandExitCode(ctx, sshClient, command)
	if err != nil {
		return false, err
	}

	return code == 0, nil
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
	runResult, err := runRemoteCommandWithInput(ctx, sshClient, command, bytes.NewReader(output))
	if err != nil {
		return false, err
	}

	return runResult == 0, nil
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

func runRemoteCommandWithInput(
	ctx context.Context,
	sshClient *ssh.Client,
	command string,
	input io.Reader,
) (int, error) {
	session, err := sshClient.NewSession()
	if err != nil {
		return -1, fmt.Errorf("create ssh session: %w", err)
	}
	defer func() { _ = session.Close() }()

	errChan, err := startRemoteInputCopy(session, input)
	if err != nil {
		return -1, err
	}
	defer drainErrChan(errChan)

	if err := session.Start(command); err != nil {
		return -1, fmt.Errorf("start remote command: %w", err)
	}

	return waitRemoteCommand(ctx, session)
}

func startRemoteInputCopy(session *ssh.Session, input io.Reader) (<-chan error, error) {
	if input == nil {
		return nil, nil
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("get remote stdin: %w", err)
	}

	return streamBytesToWriter(input, stdin), nil
}

func drainErrChan(errChan <-chan error) {
	if errChan == nil {
		return
	}

	<-errChan
}

func waitRemoteCommand(ctx context.Context, session *ssh.Session) (int, error) {
	waitErrCh := make(chan error, 1)
	go func() {
		waitErrCh <- session.Wait()
	}()

	select {
	case <-ctx.Done():
		_ = session.Close()
		return -1, fmt.Errorf("remote command canceled: %w", ctx.Err())
	case err := <-waitErrCh:
		if err == nil {
			return 0, nil
		}

		var exitErr *ssh.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitStatus(), nil
		}

		return -1, fmt.Errorf("run remote command: %w", err)
	}
}

func runRemoteCommandExitCode(ctx context.Context, sshClient *ssh.Client, command string) (int, error) {
	return runRemoteCommandWithInput(ctx, sshClient, command, nil)
}

func isValidTermMode(mode string) bool {
	switch mode {
	case "", TermModeAuto, TermModeStrict, TermModeFallback:
		return true
	default:
		return false
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
