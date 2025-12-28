package sshtunnel

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/skevetter/log"

	"github.com/pkg/errors"
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
			errChan <- fmt.Errorf("executing agent command %w", err)
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
			errChan <- fmt.Errorf("create ssh client %w", err)
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
				errChan <- fmt.Errorf("forward agent %w", err)
			}
			err = devsshagent.RequestAgentForwarding(sess)
			if err != nil {
				errChan <- fmt.Errorf("request agent forwarding failed %w", err)
			}
		}
		tunnelLogWriter := &tunnelLogWriter{logger: log}
		defer func() {
			_ = tunnelLogWriter.Close()
			log.Debug("tunnel log writer closed")
		}()

		var stderrBuf bytes.Buffer
		tunnelWriter := io.MultiWriter(&stderrBuf, tunnelLogWriter)

		log.WithFields(logrus.Fields{"command": command}).Debug("running agent command in SSH tunnel")
		err = devssh.Run(ctx, sshClient, command, gRPCConnStdinReader, gRPCConnStdoutWriter, tunnelWriter, nil)
		if err != nil {
			if stderrBuf.Len() > 0 {
				errChan <- fmt.Errorf("run agent command failed %s", stderrBuf.String())
			} else {
				errChan <- fmt.Errorf("run agent command failed %w", err)
			}
		} else {
			errChan <- nil
		}
	}()

	result, err := tunnelServerFunc(cancelCtx, gRPCConnStdinWriter, gRPCConnStdoutReader)
	if err != nil {
		return nil, fmt.Errorf("start tunnel server %w", err)
	}

	log.Debug("tunnel server started, waiting for command completion")

	// wait until command finished
	if err := <-errChan; err != nil {
		return result, err
	}

	log.Debug("SSH tunnel execution completed")
	return result, <-errChan
}

type tunnelLogWriter struct {
	logger log.Logger
	buffer bytes.Buffer
	mu     sync.Mutex
}

func (w *tunnelLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.buffer.Write(p)

	for {
		line, err := w.buffer.ReadString('\n')
		if err != nil {
			w.buffer.WriteString(line) // Put back incomplete line
			break
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if matched, level := w.extractLogLevel(line); matched {
			w.logger.Print(level, line)
		} else {
			// Default messages to debug
			w.logger.Debug(line)
		}
	}

	return len(p), nil
}

func (w *tunnelLogWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Flush remaining data in the buffer
	remaining := strings.TrimSpace(w.buffer.String())
	if remaining == "" {
		return nil
	}
	lines := strings.SplitSeq(remaining, "\n")
	for line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if matched, level := w.extractLogLevel(line); matched {
			w.logger.Print(level, line)
		} else {
			w.logger.Debug(line)
		}
	}

	return nil
}

func (w *tunnelLogWriter) extractLogLevel(line string) (bool, logrus.Level) {
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
