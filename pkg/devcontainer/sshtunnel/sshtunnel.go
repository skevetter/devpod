package sshtunnel

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	client2 "github.com/skevetter/devpod/pkg/client"
	config2 "github.com/skevetter/devpod/pkg/devcontainer/config"
	devssh "github.com/skevetter/devpod/pkg/ssh"
	devsshagent "github.com/skevetter/devpod/pkg/ssh/agent"
	"github.com/skevetter/log"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	// maxLogLines is the number of error lines to keep from tunnel output for debugging.
	maxLogLines = 1
)

type (
	AgentInjectFunc  func(context.Context, string, *os.File, *os.File, io.WriteCloser) error
	TunnelServerFunc func(ctx context.Context, stdin io.WriteCloser, stdout io.Reader) (*config2.Result, error)
)

var logLevelMap = map[string]logrus.Level{
	"debug": logrus.DebugLevel,
	"info":  logrus.InfoLevel,
	"warn":  logrus.WarnLevel,
	"error": logrus.ErrorLevel,
	"fatal": logrus.FatalLevel,
}

type ExecuteCommandOptions struct {
	Client           client2.WorkspaceClient
	AddPrivateKeys   bool
	AgentInject      AgentInjectFunc
	SSHCommand       string
	Command          string
	Log              log.Logger
	TunnelServerFunc TunnelServerFunc
}

type tunnelContext struct {
	opts      ExecuteCommandOptions
	sshPipes  *pipePair
	grpcPipes *pipePair
}

// ExecuteCommand runs the command in an SSH Tunnel and returns the result.
func ExecuteCommand(ctx context.Context, opts ExecuteCommandOptions) (*config2.Result, error) {
	if opts.TunnelServerFunc == nil {
		return nil, errors.New("tunnel server func is required")
	}
	if opts.AgentInject == nil {
		return nil, errors.New("agent inject func is required")
	}
	opts.Log.Debugf("starting SSH tunnel execution: ssh=%q workspace=%q addKeys=%v",
		opts.SSHCommand, opts.Command, opts.AddPrivateKeys)

	tc, err := setupTunnelContext(opts)
	if err != nil {
		return nil, err
	}
	defer tc.cleanup()

	g, ctx := errgroup.WithContext(ctx)
	var result *config2.Result

	// Start SSH server helper
	g.Go(func() error {
		return executeSSHServerHelper(ctx, &sshServerHelperParams{
			opts:         opts,
			stdinReader:  tc.sshPipes.stdinReader,
			stdoutWriter: tc.sshPipes.stdoutWriter,
		})
	})

	if opts.AddPrivateKeys {
		addPrivateKeys(ctx, opts)
	}

	// Start SSH tunnel
	g.Go(func() error {
		return runSSHTunnel(ctx, tc)
	})

	// Run gRPC server
	g.Go(func() error {
		var err error
		result, err = tc.opts.TunnelServerFunc(
			ctx,
			tc.grpcPipes.stdinWriter,
			tc.grpcPipes.stdoutReader,
		)
		return err
	})

	// Wait for all to complete
	if err := g.Wait(); err != nil {
		return result, err
	}

	return result, nil
}

func setupTunnelContext(opts ExecuteCommandOptions) (*tunnelContext, error) {
	sshPipes, err := createPipes()
	if err != nil {
		return nil, err
	}

	grpcPipes, err := createPipes()
	if err != nil {
		sshPipes.Close()
		return nil, err
	}

	return &tunnelContext{
		opts:      opts,
		sshPipes:  sshPipes,
		grpcPipes: grpcPipes,
	}, nil
}

func (tc *tunnelContext) cleanup() {
	tc.sshPipes.Close()
	tc.grpcPipes.Close()
}

type pipePair struct {
	stdoutReader *os.File
	stdoutWriter *os.File
	stdinReader  *os.File
	stdinWriter  *os.File
}

func (p *pipePair) Close() {
	closeFile := func(fp **os.File) {
		if *fp != nil {
			_ = (*fp).Close()
			*fp = nil
		}
	}
	closeFile(&p.stdoutReader)
	closeFile(&p.stdoutWriter)
	closeFile(&p.stdinReader)
	closeFile(&p.stdinWriter)
}

func createPipes() (*pipePair, error) {
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	stdinReader, stdinWriter, err := os.Pipe()
	if err != nil {
		_ = stdoutReader.Close()
		_ = stdoutWriter.Close()
		return nil, err
	}
	return &pipePair{
		stdoutReader: stdoutReader,
		stdoutWriter: stdoutWriter,
		stdinReader:  stdinReader,
		stdinWriter:  stdinWriter,
	}, nil
}

type sshServerHelperParams struct {
	opts         ExecuteCommandOptions
	stdinReader  *os.File
	stdoutWriter *os.File
}

func isExpectedError(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	// Check for process killed by signal
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		// Process was terminated by a signal
		return exitErr != nil && !exitErr.Exited()
	}
	return false
}

func executeSSHServerHelper(ctx context.Context, p *sshServerHelperParams) error {
	defer p.opts.Log.Debug("done executing SSH server helper command")

	writer := p.opts.Log.Writer(logrus.InfoLevel, false)
	defer func() { _ = writer.Close() }()

	p.opts.Log.Debugf("injecting and running SSH server command: %q", p.opts.SSHCommand)
	err := p.opts.AgentInject(ctx, p.opts.SSHCommand, p.stdinReader, p.stdoutWriter, writer)
	if err != nil && !isExpectedError(err) {
		return fmt.Errorf("executing agent command: %w", err)
	}
	return nil
}

func addPrivateKeys(ctx context.Context, opts ExecuteCommandOptions) {
	opts.Log.Debug("adding SSH keys to agent")
	err := devssh.AddPrivateKeysToAgent(ctx, opts.Log)
	if err != nil {
		opts.Log.Debugf("failed to add private keys to SSH agent: %v", err)
	}
}

func runSSHTunnel(ctx context.Context, tc *tunnelContext) error {
	tc.opts.Log.Debug("creating SSH client")
	sshClient, err := devssh.StdioClient(tc.sshPipes.stdoutReader, tc.sshPipes.stdinWriter, false)
	if err != nil {
		return fmt.Errorf("failed to create SSH client: %w", err)
	}
	tc.opts.Log.Debug("SSH client created")
	defer func() {
		_ = sshClient.Close()
		tc.opts.Log.Debug("SSH client closed")
	}()

	sess, err := establishSSHSession(ctx, tc, sshClient)
	if err != nil {
		return err
	}
	defer func() {
		_ = sess.Close()
		tc.opts.Log.Debug("SSH session closed")
	}()

	if err = setupSSHAgentForwarding(tc, sshClient, sess); err != nil {
		return err
	}
	return runCommandInSSHTunnel(ctx, tc, sshClient)
}

func establishSSHSession(
	ctx context.Context,
	tc *tunnelContext,
	sshClient *ssh.Client,
) (*ssh.Session, error) {
	backoff := wait.Backoff{
		Duration: 500 * time.Millisecond,
		Factor:   1.5,
		Jitter:   0.1,
		Steps:    20,
	}

	var session *ssh.Session
	if err := wait.ExponentialBackoffWithContext(
		ctx,
		backoff,
		func(ctx context.Context) (bool, error) {
			sess, err := sshClient.NewSession()
			if err != nil {
				tc.opts.Log.Debugf("SSH server not ready: %v", err)
				return false, nil // Retry
			}
			tc.opts.Log.Debug("SSH session created")
			session = sess
			return true, nil // Success
		},
	); err != nil {
		return nil, fmt.Errorf("SSH server timeout: %w", err)
	}

	return session, nil
}

func setupSSHAgentForwarding(
	tc *tunnelContext,
	sshClient *ssh.Client,
	sess *ssh.Session,
) error {
	identityAgent := devsshagent.GetSSHAuthSocket()
	if identityAgent == "" {
		return nil
	}

	tc.opts.Log.Debugf("forwarding SSH agent: socket=%s", identityAgent)

	var err error
	if err = devsshagent.ForwardToRemote(sshClient, identityAgent); err == nil {
		err = devsshagent.RequestAgentForwarding(sess)
	}

	if err != nil {
		tc.opts.Log.Warnf("SSH agent forwarding failed: %v", err)
		return fmt.Errorf("forward agent: %w", err)
	}
	return nil
}

func runCommandInSSHTunnel(ctx context.Context, tc *tunnelContext, sshClient *ssh.Client) error {
	streamer := NewTunnelLogStreamer(tc.opts.Log)
	defer func() { _ = streamer.Close() }()

	tc.opts.Log.Debugf("running agent command in SSH tunnel: %q", tc.opts.Command)
	if err := devssh.Run(devssh.RunOptions{
		Context: ctx,
		Client:  sshClient,
		Command: tc.opts.Command,
		Stdin:   tc.grpcPipes.stdinReader,
		Stdout:  tc.grpcPipes.stdoutWriter,
		Stderr:  streamer,
	}); err != nil {
		_ = streamer.Close()
		if out := streamer.ErrorOutput(); out != "" {
			return fmt.Errorf("run agent command failed: %w\n%s", err, out)
		}
		return fmt.Errorf("run agent command failed: %w", err)
	}
	return nil
}

type TunnelLogStreamer struct {
	pw     *io.PipeWriter
	logger log.Logger
	done   chan struct{}

	mu        sync.Mutex
	lastLines []string
}

func NewTunnelLogStreamer(logger log.Logger) *TunnelLogStreamer {
	pr, pw := io.Pipe()
	l := &TunnelLogStreamer{
		pw:        pw,
		logger:    logger,
		done:      make(chan struct{}),
		lastLines: make([]string, 0, maxLogLines),
	}

	go l.process(pr)
	return l
}

func (l *TunnelLogStreamer) Write(p []byte) (int, error) {
	return l.pw.Write(p)
}

func (l *TunnelLogStreamer) Close() error {
	err := l.pw.Close()
	<-l.done
	return err
}

func (l *TunnelLogStreamer) ErrorOutput() string {
	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.lastLines) == 0 {
		return ""
	}

	return strings.Join(l.lastLines, "\n")
}

func (l *TunnelLogStreamer) process(r io.Reader) {
	defer close(l.done)
	scanner := bufio.NewScanner(r)

	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		l.logLine(line)

		l.mu.Lock()
		if len(l.lastLines) >= maxLogLines {
			l.lastLines = l.lastLines[1:]
		}
		l.lastLines = append(l.lastLines, line)
		l.mu.Unlock()
	}

	if err := scanner.Err(); err != nil {
		l.logger.Debugf("error reading tunnel output: %v", err)
	}
}

func (l *TunnelLogStreamer) logLine(line string) {
	line = strings.TrimSpace(line)
	// Remove carriage returns to prevent terminal overwriting (e.g. git progress)
	line = strings.ReplaceAll(line, "\r", "")
	if line == "" {
		return
	}

	if matched, level := l.extractLogLevel(line); matched {
		l.logger.Print(level, line)
	} else {
		l.logger.Debug(line)
	}
}

func (l *TunnelLogStreamer) extractLogLevel(line string) (bool, logrus.Level) {
	parts := strings.SplitN(line, " ", 3)
	if len(parts) < 2 || !strings.Contains(parts[0], ":") {
		return false, logrus.DebugLevel
	}

	level, ok := logLevelMap[strings.ToLower(parts[1])]

	return ok, level
}
