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
)

type AgentInjectFunc func(context.Context, string, *os.File, *os.File, io.WriteCloser) error
type TunnelServerFunc func(ctx context.Context, stdin io.WriteCloser, stdout io.Reader) (*config2.Result, error)

// ExecuteCommand runs the command in an SSH Tunnel and returns the result.
func ExecuteCommand(
	ctx context.Context,
	client client2.WorkspaceClient,
	addPrivateKeys bool,
	agentInject AgentInjectFunc,
	sshCommand,
	command string,
	log log.Logger,
	tunnelServerFunc TunnelServerFunc,
) (*config2.Result, error) {
	log.WithFields(logrus.Fields{
		"sshCommand":       sshCommand,
		"workspaceCommand": command,
		"addKeys":          addPrivateKeys,
	}).Debug("starting SSH tunnel execution")

	// create pipes
	sshTunnelStdoutReader, sshTunnelStdoutWriter, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	sshTunnelStdinReader, sshTunnelStdinWriter, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer func() { _ = sshTunnelStdoutWriter.Close() }()
	defer func() { _ = sshTunnelStdinWriter.Close() }()

	// start machine on stdio
	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errChan := make(chan error, 2)
	go func() {
		defer log.Debug("done executing SSH server helper command")
		defer cancel()

		writer := log.Writer(logrus.InfoLevel, false)
		defer func() { _ = writer.Close() }()

		log.WithFields(logrus.Fields{"command": sshCommand}).Debug("inject and run command")
		err := agentInject(ctx, sshCommand, sshTunnelStdinReader, sshTunnelStdoutWriter, writer)
		if err != nil && !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "signal: ") {
			errChan <- fmt.Errorf("executing agent command: %w", err)
		} else {
			errChan <- nil
		}
	}()

	if addPrivateKeys {
		log.Debug("adding SSH keys to agent, disable via 'devpod context set-options -o SSH_ADD_PRIVATE_KEYS=false'")
		err := devssh.AddPrivateKeysToAgent(ctx, log)
		if err != nil {
			log.WithFields(logrus.Fields{"error": err}).Debug("error adding private keys to SSH agent")
		}
	}

	// create pipes
	gRPCConnStdoutReader, gRPCConnStdoutWriter, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer func() { _ = gRPCConnStdoutWriter.Close() }()
	gRPCConnStdinReader, gRPCConnStdinWriter, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer func() { _ = gRPCConnStdinWriter.Close() }()

	// connect to container
	go func() {
		defer cancel()

		log.Debug("attempting to create SSH client")
		// start ssh client as root / default user
		sshClient, err := devssh.StdioClient(sshTunnelStdoutReader, sshTunnelStdinWriter, false)
		if err != nil {
			errChan <- fmt.Errorf("create ssh client: %w", err)
			return
		}
		log.Debugf("SSH client created")
		defer func() {
			_ = sshClient.Close()
			log.Debug("SSH client closed")
		}()
		var sess *ssh.Session
		timeout := time.After(60 * time.Second)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-timeout:
				errChan <- fmt.Errorf("SSH server timeout after %d seconds", 60)
				return
			case <-cancelCtx.Done():
				errChan <- cancelCtx.Err()
				return
			case <-ticker.C:
				var err error
				sess, err = sshClient.NewSession()
				if err != nil {
					log.WithFields(logrus.Fields{"error": err}).Debug("SSH server not ready")
					continue
				}
				goto sessionReady
			}
		}

	sessionReady:
		log.Debug("SSH session created")
		defer func() {
			_ = sess.Close()
			log.Debug("SSH session closed")
		}()
		identityAgent := devsshagent.GetSSHAuthSocket()
		if identityAgent != "" {
			log.WithFields(logrus.Fields{"socket": identityAgent}).Debug("forwarding SSH agent")
			err = devsshagent.ForwardToRemote(sshClient, identityAgent)
			if err != nil {
				errChan <- fmt.Errorf("forward agent: %w", err)
			}
			err = devsshagent.RequestAgentForwarding(sess)
			if err != nil {
				errChan <- fmt.Errorf("request agent forwarding failed: %w", err)
			}
		}

		streamer := NewTunnelLogStreamer(log)
		defer func() { _ = streamer.Close() }()

		log.WithFields(logrus.Fields{"command": command}).Debug("running agent command in SSH tunnel")
		if err := devssh.Run(devssh.RunOptions{
			Context: ctx,
			Client:  sshClient,
			Command: command,
			Stdin:   gRPCConnStdinReader,
			Stdout:  gRPCConnStdoutWriter,
			Stderr:  streamer,
		}); err != nil {
			_ = streamer.Close()
			if out := streamer.ErrorOutput(); out != "" {
				errChan <- fmt.Errorf("run agent command failed: %w\n%s", err, out)
			} else {
				errChan <- fmt.Errorf("run agent command failed: %w", err)
			}
		} else {
			errChan <- nil
		}
	}()

	result, err := tunnelServerFunc(cancelCtx, gRPCConnStdinWriter, gRPCConnStdoutReader)
	if err != nil {
		return nil, fmt.Errorf("start tunnel server: %w", err)
	}

	log.Debug("tunnel server started, waiting for command completion")

	// wait until command finished
	if err := <-errChan; err != nil {
		return result, err
	}

	log.Debug("SSH tunnel execution completed")
	return result, <-errChan
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
		l.logger.WithFields(logrus.Fields{"error": err}).Debug("error reading tunnel output")
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
	if len(parts) < 2 {
		return false, logrus.DebugLevel
	}

	if !strings.Contains(parts[0], ":") {
		return false, logrus.DebugLevel
	}

	switch parts[1] {
	case "debug":
		return true, logrus.DebugLevel
	case "info":
		return true, logrus.InfoLevel
	case "warn":
		return true, logrus.WarnLevel
	case "error":
		return true, logrus.ErrorLevel
	case "fatal":
		return true, logrus.FatalLevel
	}

	return false, logrus.DebugLevel
}
