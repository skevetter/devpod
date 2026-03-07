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
	Ctx              context.Context
	Client           client2.WorkspaceClient
	AddPrivateKeys   bool
	AgentInject      AgentInjectFunc
	SSHCommand       string
	Command          string
	Log              log.Logger
	TunnelServerFunc TunnelServerFunc
}

// sshTunnelResult carries the outcome of a single goroutine so the caller
// knows which component reported the error.
type sshTunnelResult struct {
	source string
	err    error
}

type tunnelContext struct {
	opts      ExecuteCommandOptions
	cancelCtx context.Context
	cancel    context.CancelFunc

	helperDone chan sshTunnelResult
	tunnelDone chan sshTunnelResult

	sshPipes  *pipePair
	grpcPipes *pipePair
}

// ExecuteCommand runs the command in an SSH Tunnel and returns the result.
func ExecuteCommand(opts ExecuteCommandOptions) (*config2.Result, error) {
	opts.Log.Debugf("starting SSH tunnel execution: ssh=%q workspace=%q addKeys=%v",
		opts.SSHCommand, opts.Command, opts.AddPrivateKeys)

	tc, err := setupTunnelContext(opts)
	if err != nil {
		return nil, err
	}
	defer tc.cleanup()

	var wg sync.WaitGroup

	wg.Go(func() {
		tc.helperDone <- executeSSHServerHelper(tc)
	})

	if opts.AddPrivateKeys {
		addPrivateKeys(opts)
	}

	wg.Go(func() {
		tc.tunnelDone <- runSSHTunnel(tc)
	})

	result, err := waitForTunnelCompletion(tc)

	// Wait for goroutines to complete
	wg.Wait()

	return result, err
}

func setupTunnelContext(opts ExecuteCommandOptions) (*tunnelContext, error) {
	sshPipes, err := createPipes()
	if err != nil {
		return nil, err
	}

	cancelCtx, cancel := context.WithCancel(opts.Ctx)

	grpcPipes, err := createPipes()
	if err != nil {
		sshPipes.Close()
		cancel()
		return nil, err
	}

	return &tunnelContext{
		opts:       opts,
		cancelCtx:  cancelCtx,
		cancel:     cancel,
		helperDone: make(chan sshTunnelResult, 1),
		tunnelDone: make(chan sshTunnelResult, 1),
		sshPipes:   sshPipes,
		grpcPipes:  grpcPipes,
	}, nil
}

func (tc *tunnelContext) cleanup() {
	tc.sshPipes.Close()
	tc.grpcPipes.Close()
	tc.cancel()
}

// cleanupTimeout is how long waitForTunnelCompletion waits for goroutines
// to report after the tunnel server has returned a valid result.
const cleanupTimeout = 10 * time.Second

func waitForTunnelCompletion(tc *tunnelContext) (*config2.Result, error) {
	result, err := tc.opts.TunnelServerFunc(
		tc.cancelCtx,
		tc.grpcPipes.stdinWriter,
		tc.grpcPipes.stdoutReader,
	)
	if err != nil {
		return nil, fmt.Errorf("tunnel server: %w", err)
	}

	tc.opts.Log.Debug("awaiting tunnel server command completion")

	// Collect results from both goroutines. When the tunnel server already
	// has a valid result, EOF from the tunnel goroutine is expected (the SSH
	// session closes because the agent finished). We still treat helper
	// errors and non-EOF tunnel errors as real failures.
	var errs []error
	timer := time.NewTimer(cleanupTimeout)
	defer timer.Stop()

	for range 2 {
		select {
		case r := <-tc.helperDone:
			if r.err != nil {
				tc.opts.Log.Debugf("helper goroutine error: %v", r.err)
				errs = append(errs, r.err)
			}
		case r := <-tc.tunnelDone:
			if r.err != nil && (result == nil || !errors.Is(r.err, io.EOF)) {
				tc.opts.Log.Debugf("tunnel goroutine error: %v", r.err)
				errs = append(errs, r.err)
			}
		case <-timer.C:
			tc.opts.Log.Debug("timed out waiting for goroutines after successful result")
			return result, errors.Join(errs...)
		}
	}

	tc.opts.Log.Debug("SSH tunnel execution completed")

	if len(errs) == 0 {
		return result, nil
	}
	return result, errors.Join(errs...)
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
func executeSSHServerHelper(tc *tunnelContext) sshTunnelResult {
	defer tc.opts.Log.Debug("done executing SSH server helper command")
	defer tc.cancel()

	writer := tc.opts.Log.Writer(logrus.InfoLevel, false)
	defer func() { _ = writer.Close() }()

	tc.opts.Log.Debugf("injecting and running SSH server command: %q", tc.opts.SSHCommand)
	err := tc.opts.AgentInject(
		tc.opts.Ctx,
		tc.opts.SSHCommand,
		tc.sshPipes.stdinReader,
		tc.sshPipes.stdoutWriter,
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

func addPrivateKeys(opts ExecuteCommandOptions) {
	opts.Log.Debug("adding SSH keys to agent")
	err := devssh.AddPrivateKeysToAgent(opts.Ctx, opts.Log)
	if err != nil {
		opts.Log.Debugf("failed to add private keys to SSH agent: %v", err)
	}
}

// runSSHTunnel creates the SSH client, establishes a session, and runs the
// agent command. It returns a single goroutineResult; the caller sends it to
// tunnelDone.
func runSSHTunnel(tc *tunnelContext) sshTunnelResult {
	defer tc.cancel()

	tc.opts.Log.Debug("creating SSH client")
	sshClient, err := devssh.StdioClient(tc.sshPipes.stdoutReader, tc.sshPipes.stdinWriter, false)
	if err != nil {
		return sshTunnelResult{
			source: "tunnel",
			err:    fmt.Errorf("failed to create SSH client: %w", err),
		}
	}
	tc.opts.Log.Debug("SSH client created")
	defer func() {
		_ = sshClient.Close()
		tc.opts.Log.Debug("SSH client closed")
	}()

	sess, err := establishSSHSession(tc, sshClient)
	if err != nil {
		return sshTunnelResult{source: "tunnel", err: err}
	}
	defer func() {
		_ = sess.Close()
		tc.opts.Log.Debug("SSH session closed")
	}()

	if err = setupSSHAgentForwarding(tc, sshClient, sess); err != nil {
		return sshTunnelResult{source: "tunnel", err: fmt.Errorf("forward agent: %w", err)}
	}

	return runCommandInSSHTunnel(tc, sshClient)
}

func establishSSHSession(
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
		tc.cancelCtx,
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
	}
	return err
}

// runCommandInSSHTunnel runs the agent command over the SSH tunnel and returns
// the result. EOF errors preserve the underlying io.EOF so the caller can
// distinguish expected session closure from real failures.
func runCommandInSSHTunnel(tc *tunnelContext, sshClient *ssh.Client) sshTunnelResult {
	streamer := NewTunnelLogStreamer(tc.opts.Log)
	defer func() { _ = streamer.Close() }()

	tc.opts.Log.Debugf("running agent command in SSH tunnel: %q", tc.opts.Command)
	err := devssh.Run(devssh.RunOptions{
		Context: tc.cancelCtx,
		Client:  sshClient,
		Command: tc.opts.Command,
		Stdin:   tc.grpcPipes.stdinReader,
		Stdout:  tc.grpcPipes.stdoutWriter,
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
