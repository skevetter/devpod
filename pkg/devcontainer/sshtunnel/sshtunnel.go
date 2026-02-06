package sshtunnel

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/skevetter/log"

	"errors"

	"github.com/sirupsen/logrus"
	client2 "github.com/skevetter/devpod/pkg/client"
	config2 "github.com/skevetter/devpod/pkg/devcontainer/config"
	devssh "github.com/skevetter/devpod/pkg/ssh"
	devsshagent "github.com/skevetter/devpod/pkg/ssh/agent"
	"golang.org/x/crypto/ssh"
	"k8s.io/apimachinery/pkg/util/wait"
)

type AgentInjectFunc func(context.Context, string, *os.File, *os.File, io.WriteCloser) error
type TunnelServerFunc func(ctx context.Context, stdin io.WriteCloser, stdout io.Reader) (*config2.Result, error)

type ExecuteCommandOptions struct {
	Ctx              context.Context
	Client           client2.WorkspaceClient
	AddPrivateKeys   bool
	AgentInject      AgentInjectFunc
	SSHCommand       string
	Command          string
	Log              log.Logger
	TunnelServerFunc TunnelServerFunc
	SSHTimeout       time.Duration // Default: 60s
	SSHRetryInterval time.Duration // Default: 500ms
}

type tunnelContext struct {
	opts             ExecuteCommandOptions
	cancelCtx        context.Context
	cancel           context.CancelFunc
	errChan          chan error
	sshStdoutReader  *os.File
	sshStdoutWriter  *os.File
	sshStdinReader   *os.File
	sshStdinWriter   *os.File
	grpcStdoutReader *os.File
	grpcStdoutWriter *os.File
	grpcStdinReader  *os.File
	grpcStdinWriter  *os.File
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
		executeSSHServerHelper(&sshServerHelperParams{
			opts:         opts,
			cancel:       tc.cancel,
			stdinReader:  tc.sshStdinReader,
			stdoutWriter: tc.sshStdoutWriter,
			errChan:      tc.errChan,
		})
	})

	if opts.AddPrivateKeys {
		addPrivateKeys(opts)
	}

	wg.Go(func() {
		runSSHTunnel(tc)
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
	errChan := make(chan error, 2)

	grpcPipes, err := createPipes()
	if err != nil {
		cancel()
		return nil, err
	}

	return &tunnelContext{
		opts:             opts,
		cancelCtx:        cancelCtx,
		cancel:           cancel,
		errChan:          errChan,
		sshStdoutReader:  sshPipes.stdoutReader,
		sshStdoutWriter:  sshPipes.stdoutWriter,
		sshStdinReader:   sshPipes.stdinReader,
		sshStdinWriter:   sshPipes.stdinWriter,
		grpcStdoutReader: grpcPipes.stdoutReader,
		grpcStdoutWriter: grpcPipes.stdoutWriter,
		grpcStdinReader:  grpcPipes.stdinReader,
		grpcStdinWriter:  grpcPipes.stdinWriter,
	}, nil
}

func (tc *tunnelContext) cleanup() {
	_ = tc.sshStdoutReader.Close()
	_ = tc.sshStdoutWriter.Close()
	_ = tc.sshStdinReader.Close()
	_ = tc.sshStdinWriter.Close()
	_ = tc.grpcStdoutReader.Close()
	_ = tc.grpcStdoutWriter.Close()
	_ = tc.grpcStdinReader.Close()
	_ = tc.grpcStdinWriter.Close()
	tc.cancel()
}

func waitForTunnelCompletion(tc *tunnelContext) (*config2.Result, error) {
	result, err := tc.opts.TunnelServerFunc(tc.cancelCtx, tc.grpcStdinWriter, tc.grpcStdoutReader)
	if err != nil {
		return nil, fmt.Errorf("start tunnel server: %w", err)
	}

	tc.opts.Log.Debug("tunnel server started, waiting for command completion")

	// Collect both errors to handle race condition
	var errs []error
	err1 := <-tc.errChan
	if err1 != nil {
		errs = append(errs, err1)
	}
	err2 := <-tc.errChan
	if err2 != nil {
		errs = append(errs, err2)
	}

	tc.opts.Log.Debug("SSH tunnel execution completed")

	// Return first error or combine multiple errors
	if len(errs) == 0 {
		return result, nil
	}
	if len(errs) == 1 {
		return result, errs[0]
	}
	return result, fmt.Errorf("multiple errors: %w; %v", errs[0], errs[1])
}

type pipePair struct {
	stdoutReader *os.File
	stdoutWriter *os.File
	stdinReader  *os.File
	stdinWriter  *os.File
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
	cancel       context.CancelFunc
	stdinReader  *os.File
	stdoutWriter *os.File
	errChan      chan error
}

func isExpectedError(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	// Check for signal/termination errors
	return strings.Contains(err.Error(), "signal:")
}

func executeSSHServerHelper(p *sshServerHelperParams) {
	defer p.opts.Log.Debug("done executing SSH server helper command")
	defer p.cancel()

	writer := p.opts.Log.Writer(logrus.InfoLevel, false)
	defer func() { _ = writer.Close() }()

	p.opts.Log.Debugf("injecting and running SSH server command: %q", p.opts.SSHCommand)
	err := p.opts.AgentInject(p.opts.Ctx, p.opts.SSHCommand, p.stdinReader, p.stdoutWriter, writer)
	if err != nil && !isExpectedError(err) {
		p.errChan <- fmt.Errorf("executing agent command: %w", err)
	} else {
		p.errChan <- nil
	}
}

func addPrivateKeys(opts ExecuteCommandOptions) {
	opts.Log.Debug("adding SSH keys to agent")
	err := devssh.AddPrivateKeysToAgent(opts.Ctx, opts.Log)
	if err != nil {
		opts.Log.Debugf("failed to add private keys to SSH agent: %v", err)
	}
}

func runSSHTunnel(tc *tunnelContext) {
	defer tc.cancel()

	tc.opts.Log.Debug("creating SSH client")
	sshClient, err := devssh.StdioClient(tc.sshStdoutReader, tc.sshStdinWriter, false)
	if err != nil {
		tc.errChan <- fmt.Errorf("failed to create SSH client (check network and SSH server): %w", err)
		return
	}
	tc.opts.Log.Debug("SSH client created")
	defer func() {
		_ = sshClient.Close()
		tc.opts.Log.Debug("SSH client closed")
	}()

	sess, err := waitForSSHSession(tc, sshClient)
	if err == nil {
		defer func() {
			_ = sess.Close()
			tc.opts.Log.Debug("SSH session closed")
		}()

		err = setupSSHAgentForwarding(tc, sshClient, sess)
		if err == nil {
			runCommandInSSHTunnel(tc, sshClient)
		}
	}
}

func getSSHTimeout(opts ExecuteCommandOptions) time.Duration {
	if opts.SSHTimeout == 0 {
		return 60 * time.Second
	}
	return opts.SSHTimeout
}

func getSSHBackoff(opts ExecuteCommandOptions) wait.Backoff {
	duration := opts.SSHRetryInterval
	if duration == 0 {
		duration = 500 * time.Millisecond
	}

	timeout := opts.SSHTimeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	// Calculate steps to reach timeout with exponential backoff
	// Factor 1.5, starting at 500ms: 500ms, 750ms, 1125ms, 1687ms, 2531ms...
	// ~20 steps reaches ~60s total
	steps := max(10, int(timeout/duration*2))

	return wait.Backoff{
		Duration: duration,
		Factor:   1.5,
		Jitter:   0.1,
		Steps:    steps,
	}
}

func waitForSSHSession(
	tc *tunnelContext,
	sshClient *ssh.Client,
) (*ssh.Session, error) {
	backoff := getSSHBackoff(tc.opts)

	var session *ssh.Session
	if err := wait.ExponentialBackoffWithContext(tc.cancelCtx, backoff, func(ctx context.Context) (bool, error) {
		sess, err := sshClient.NewSession()
		if err != nil {
			tc.opts.Log.Debugf("SSH server not ready: %v", err)
			return false, nil // Retry
		}
		tc.opts.Log.Debug("SSH session created")
		session = sess
		return true, nil // Success
	}); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			tc.errChan <- err
			return nil, err
		}
		// Timeout from backoff
		timeoutErr := fmt.Errorf("SSH server timeout after %v", getSSHTimeout(tc.opts))
		tc.errChan <- timeoutErr
		return nil, timeoutErr
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
		tc.errChan <- fmt.Errorf("forward agent: %w", err)
	}
	return err
}

func runCommandInSSHTunnel(tc *tunnelContext, sshClient *ssh.Client) {
	streamer := NewTunnelLogStreamer(tc.opts.Log)
	defer func() { _ = streamer.Close() }()

	tc.opts.Log.Debugf("running agent command in SSH tunnel: %q", tc.opts.Command)
	if err := devssh.Run(devssh.RunOptions{
		Context: tc.opts.Ctx,
		Client:  sshClient,
		Command: tc.opts.Command,
		Stdin:   tc.grpcStdinReader,
		Stdout:  tc.grpcStdoutWriter,
		Stderr:  streamer,
	}); err != nil {
		_ = streamer.Close()
		if out := streamer.ErrorOutput(); out != "" {
			tc.errChan <- fmt.Errorf("run agent command failed: %w\n%s", err, out)
		} else {
			tc.errChan <- fmt.Errorf("run agent command failed: %w", err)
		}
	} else {
		tc.errChan <- nil
	}
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

	level, ok := map[string]logrus.Level{
		"debug": logrus.DebugLevel,
		"info":  logrus.InfoLevel,
		"warn":  logrus.WarnLevel,
		"error": logrus.ErrorLevel,
		"fatal": logrus.FatalLevel,
	}[parts[1]]

	return ok, level
}
