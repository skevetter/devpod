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
	"k8s.io/apimachinery/pkg/util/wait"
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

// sshTunnelResult carries the result of a single goroutine so the caller
// knows which component reported the error.
type sshTunnelResult struct {
	source string
	err    error
}

type sshSessionTunnel struct {
	opts ExecuteCommandOptions

	helperDone chan sshTunnelResult
	tunnelDone chan sshTunnelResult

	sshPipes  *pipePair
	grpcPipes *pipePair
}

// ExecuteCommand runs the command in an SSH Tunnel and returns the result.
func ExecuteCommand(ctx context.Context, opts ExecuteCommandOptions) (*config2.Result, error) {
	opts.Log.Debugf("starting SSH tunnel execution: ssh=%q workspace=%q addKeys=%v",
		opts.SSHCommand, opts.Command, opts.AddPrivateKeys)

	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	ts, err := setupTunnelContext(opts)
	if err != nil {
		return nil, err
	}
	defer ts.cleanup()

	var wg sync.WaitGroup

	wg.Go(func() {
		// helper receives parent ctx; it calls cancel() to signal tunnel goroutine
		ts.helperDone <- executeSSHServerHelper(ctx, cancel, ts)
	})

	if opts.AddPrivateKeys {
		addPrivateKeys(cancelCtx, opts)
	}

	wg.Go(func() {
		ts.tunnelDone <- runSSHTunnel(cancelCtx, cancel, ts)
	})

	result, err := waitForTunnelCompletion(cancelCtx, ts)
	wg.Wait()

	return result, err
}

func setupTunnelContext(opts ExecuteCommandOptions) (*sshSessionTunnel, error) {
	sshPipes, err := createPipes()
	if err != nil {
		return nil, err
	}

	grpcPipes, err := createPipes()
	if err != nil {
		sshPipes.Close()
		return nil, err
	}

	return &sshSessionTunnel{
		opts:       opts,
		helperDone: make(chan sshTunnelResult, 1),
		tunnelDone: make(chan sshTunnelResult, 1),
		sshPipes:   sshPipes,
		grpcPipes:  grpcPipes,
	}, nil
}

func (ts *sshSessionTunnel) cleanup() {
	ts.sshPipes.Close()
	ts.grpcPipes.Close()
}

// cleanupTimeout is how long waitForTunnelCompletion waits for goroutines
// to report after the tunnel server has returned a valid result.
const cleanupTimeout = 10 * time.Second

func waitForTunnelCompletion(ctx context.Context, ts *sshSessionTunnel) (*config2.Result, error) {
	result, err := ts.opts.TunnelServerFunc(
		ctx,
		ts.grpcPipes.stdinWriter,
		ts.grpcPipes.stdoutReader,
	)
	if err != nil {
		return nil, fmt.Errorf("tunnel server: %w", err)
	}

	ts.opts.Log.Debug("awaiting tunnel server command completion")
	errs := collectTunnelErrors(ts, result)
	ts.opts.Log.Debug("SSH tunnel execution completed")

	return result, errors.Join(errs...)
}

// collectTunnelErrors drains helperDone and tunnelDone, returning any
// real errors. EOF from the tunnel goroutine is ignored when a valid result
// already exists.
func collectTunnelErrors(
	ts *sshSessionTunnel,
	result *config2.Result,
) []error {
	var errs []error
	timer := time.NewTimer(cleanupTimeout)
	defer timer.Stop()

	for range 2 {
		select {
		case r := <-ts.helperDone:
			if r.err != nil {
				ts.opts.Log.Debugf("helper goroutine error: %v", r.err)
				errs = append(errs, r.err)
			}
		case r := <-ts.tunnelDone:
			if isTunnelError(r.err, result) {
				ts.opts.Log.Debugf("tunnel goroutine error: %v", r.err)
				errs = append(errs, r.err)
			}
		case <-timer.C:
			ts.opts.Log.Debug("timed out waiting for goroutines after successful result")

			return errs
		}
	}

	return errs
}

// isTunnelError reports whether a tunnel goroutine error is a real failure.
// EOF is expected when a valid result exists (the SSH session closes normally).
func isTunnelError(err error, result *config2.Result) bool {
	return err != nil && (result == nil || !errors.Is(err, io.EOF))
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

func isExpectedError(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr != nil && !exitErr.Exited()
	}
	return false
}

// executeSSHServerHelper injects the agent and runs the SSH server helper
// command. It returns a single goroutineResult; the caller sends it to
// helperDone.
func executeSSHServerHelper(
	ctx context.Context,
	cancel context.CancelFunc,
	ts *sshSessionTunnel,
) sshTunnelResult {
	defer ts.opts.Log.Debug("done executing SSH server helper command")
	defer cancel()

	writer := ts.opts.Log.Writer(logrus.InfoLevel, false)
	defer func() { _ = writer.Close() }()

	ts.opts.Log.Debugf("injecting and running SSH server command: %q", ts.opts.SSHCommand)
	err := ts.opts.AgentInject(
		ctx,
		ts.opts.SSHCommand,
		ts.sshPipes.stdinReader,
		ts.sshPipes.stdoutWriter,
		writer,
	)
	if err != nil && !isExpectedError(err) {
		return sshTunnelResult{
			source: "helper",
			err:    fmt.Errorf("executing agent command: %w", err),
		}
	}
	return sshTunnelResult{source: "helper"}
}

func addPrivateKeys(ctx context.Context, opts ExecuteCommandOptions) {
	opts.Log.Debug("adding SSH keys to agent")
	err := devssh.AddPrivateKeysToAgent(ctx, opts.Log)
	if err != nil {
		opts.Log.Debugf("failed to add private keys to SSH agent: %v", err)
	}
}

// runSSHTunnel creates the SSH client, establishes a session, and runs the
// agent command. It returns a single goroutineResult; the caller sends it to
// tunnelDone.
func runSSHTunnel(
	ctx context.Context,
	cancel context.CancelFunc,
	ts *sshSessionTunnel,
) sshTunnelResult {
	defer cancel()

	ts.opts.Log.Debug("creating SSH client")
	sshClient, err := devssh.StdioClient(ts.sshPipes.stdoutReader, ts.sshPipes.stdinWriter, false)
	if err != nil {
		return sshTunnelResult{
			source: "tunnel",
			err:    fmt.Errorf("failed to create SSH client: %w", err),
		}
	}
	ts.opts.Log.Debug("SSH client created")
	defer func() {
		_ = sshClient.Close()
		ts.opts.Log.Debug("SSH client closed")
	}()

	sess, err := establishSSHSession(ctx, ts, sshClient)
	if err != nil {
		return sshTunnelResult{source: "tunnel", err: err}
	}
	defer func() {
		_ = sess.Close()
		ts.opts.Log.Debug("SSH session closed")
	}()

	if err = setupSSHAgentForwarding(ts, sshClient, sess); err != nil {
		return sshTunnelResult{source: "tunnel", err: fmt.Errorf("forward agent: %w", err)}
	}

	return runCommandInSSHTunnel(ctx, ts, sshClient)
}

func establishSSHSession(
	ctx context.Context,
	ts *sshSessionTunnel,
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
				ts.opts.Log.Debugf("SSH server not ready: %v", err)
				return false, nil // Retry
			}
			ts.opts.Log.Debug("SSH session created")
			session = sess
			return true, nil // Success
		},
	); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, fmt.Errorf("SSH server timeout: %w", err)
	}

	return session, nil
}

// setupSSHAgentForwarding configures SSH agent forwarding on the session.
// Errors are returned to the caller rather than sent to a channel directly.
func setupSSHAgentForwarding(
	ts *sshSessionTunnel,
	sshClient *ssh.Client,
	sess *ssh.Session,
) error {
	identityAgent := devsshagent.GetSSHAuthSocket()
	if identityAgent == "" {
		return nil
	}

	ts.opts.Log.Debugf("forwarding SSH agent: socket=%s", identityAgent)

	var err error
	if err = devsshagent.ForwardToRemote(sshClient, identityAgent); err == nil {
		err = devsshagent.RequestAgentForwarding(sess)
	}

	if err != nil {
		ts.opts.Log.Warnf("SSH agent forwarding failed: %v", err)
	}
	return err
}

// runCommandInSSHTunnel runs the agent command over the SSH tunnel and returns
// the result. EOF errors preserve the underlying io.EOF so the caller can
// distinguish expected session closure from real failures.
func runCommandInSSHTunnel(
	ctx context.Context,
	ts *sshSessionTunnel,
	sshClient *ssh.Client,
) sshTunnelResult {
	streamer := NewTunnelLogStreamer(ts.opts.Log)
	defer func() { _ = streamer.Close() }()

	ts.opts.Log.Debugf("running agent command in SSH tunnel: %q", ts.opts.Command)
	err := devssh.Run(ctx, devssh.RunOptions{
		Client:  sshClient,
		Command: ts.opts.Command,
		Stdin:   ts.grpcPipes.stdinReader,
		Stdout:  ts.grpcPipes.stdoutWriter,
		Stderr:  streamer,
	})
	if err != nil {
		_ = streamer.Close()
		if out := streamer.ErrorOutput(); out != "" {
			return sshTunnelResult{
				source: "tunnel",
				err:    fmt.Errorf("run agent command failed: %w\n%s", err, out),
			}
		}
		return sshTunnelResult{
			source: "tunnel",
			err:    fmt.Errorf("run agent command failed: %w", err),
		}
	}
	return sshTunnelResult{source: "tunnel"}
}

const maxLogLines = 1

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
