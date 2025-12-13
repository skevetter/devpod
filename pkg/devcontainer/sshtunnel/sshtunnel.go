package sshtunnel

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/loft-sh/log"

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

type PipeManager struct {
	sshStdoutReader, sshStdoutWriter   *os.File
	sshStdinReader, sshStdinWriter     *os.File
	grpcStdoutReader, grpcStdoutWriter *os.File
	grpcStdinReader, grpcStdinWriter   *os.File
	closed                             atomic.Bool
}

func NewPipeManager() (*PipeManager, error) {
	pm := &PipeManager{}

	var err error
	pm.sshStdoutReader, pm.sshStdoutWriter, err = os.Pipe()
	if err != nil {
		return nil, err
	}

	pm.sshStdinReader, pm.sshStdinWriter, err = os.Pipe()
	if err != nil {
		_ = pm.sshStdoutWriter.Close()
		_ = pm.sshStdoutReader.Close()
		return nil, err
	}

	pm.grpcStdoutReader, pm.grpcStdoutWriter, err = os.Pipe()
	if err != nil {
		pm.Close()
		return nil, err
	}

	pm.grpcStdinReader, pm.grpcStdinWriter, err = os.Pipe()
	if err != nil {
		pm.Close()
		return nil, err
	}

	return pm, nil
}

func (pm *PipeManager) Close() {
	if pm.closed.CompareAndSwap(false, true) {
		pipes := []*os.File{
			pm.sshStdoutReader, pm.sshStdoutWriter,
			pm.sshStdinReader, pm.sshStdinWriter,
			pm.grpcStdoutReader, pm.grpcStdoutWriter,
			pm.grpcStdinReader, pm.grpcStdinWriter,
		}
		for _, pipe := range pipes {
			if pipe != nil {
				_ = pipe.Close()
			}
		}
	}
}

// SSHClientManager manages SSH client lifecycle with readiness checking
type SSHClientManager struct {
	client  *ssh.Client
	session *ssh.Session
	log     log.Logger
}

func NewSSHClientManager(stdout *os.File, stdin *os.File, log log.Logger) (*SSHClientManager, error) {
	client, err := devssh.StdioClient(stdout, stdin, false)
	if err != nil {
		return nil, fmt.Errorf("create ssh client %w", err)
	}

	return &SSHClientManager{
		client: client,
		log:    log,
	}, nil
}

func (scm *SSHClientManager) WaitForReady(ctx context.Context, timeout time.Duration) error {
	scm.log.Debugf("Waiting for SSH server to be ready")

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("SSH server not ready after %v %w", timeout, ctx.Err())
		case <-ticker.C:
			sess, err := scm.client.NewSession()
			if err == nil {
				scm.session = sess
				scm.log.Debugf("SSH session created")
				return nil
			}
			scm.log.Debugf("SSH server not ready %v", err)
		}
	}
}

func (scm *SSHClientManager) Close() {
	if scm.session != nil {
		_ = scm.session.Close()
	}
	if scm.client != nil {
		_ = scm.client.Close()
	}
}

// ExecuteCommand runs the command in an SSH Tunnel and returns the result
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
	// Setup pipes
	pipes, err := NewPipeManager()
	if err != nil {
		return nil, fmt.Errorf("create pipes %w", err)
	}
	defer pipes.Close()

	// Start agent injection
	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	injectionDone := make(chan error, 1)
	go func() {
		defer log.Debugf("Done executing ssh server helper command")
		defer cancel()

		writer := log.Writer(logrus.InfoLevel, false)
		defer func() { _ = writer.Close() }()

		log.Debugf("Inject and run command: %s", sshCommand)
		err := agentInject(cancelCtx, sshCommand, pipes.sshStdinReader, pipes.sshStdoutWriter, writer)
		if err != nil && !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "signal: ") {
			injectionDone <- fmt.Errorf("executing agent command %w", err)
		} else {
			injectionDone <- nil
		}
	}()

	// Add SSH keys to agent if required
	if addPrivateKeys {
		log.Debug("adding ssh keys to agent, disable via 'devpod context set-options -o SSH_ADD_PRIVATE_KEYS=false'")
		err := devssh.AddPrivateKeysToAgent(ctx, log)
		if err != nil {
			log.Debugf("error adding private keys to ssh-agent %v", err)
		}
	}

	// Create SSH client
	log.Debugf("attempting to create SSH client")
	sshManager, err := NewSSHClientManager(pipes.sshStdoutReader, pipes.sshStdinWriter, log)
	if err != nil {
		return nil, fmt.Errorf("create ssh client manager %w", err)
	}
	defer func() {
		log.Debugf("connection to SSH Server closed")
		sshManager.Close()
	}()

	// Wait for SSH server to be ready
	if err := sshManager.WaitForReady(cancelCtx, 60*time.Second); err != nil {
		return nil, fmt.Errorf("SSH server readiness check failed %w", err)
	}

	// Setup SSH agent forwarding
	identityAgent := devsshagent.GetSSHAuthSocket()
	if identityAgent != "" {
		log.Debugf("forwarding ssh-agent using %s", identityAgent)
		err = devsshagent.ForwardToRemote(sshManager.client, identityAgent)
		if err != nil {
			return nil, fmt.Errorf("forward agent %w", err)
		}
		err = devsshagent.RequestAgentForwarding(sshManager.session)
		if err != nil {
			return nil, fmt.Errorf("request agent forwarding failed %w", err)
		}
	}

	// Setup session I/O
	stdoutWriter := log.Writer(logrus.InfoLevel, false)
	defer func() { _ = stdoutWriter.Close() }()

	var stderrBuf bytes.Buffer
	stderrWriter := io.MultiWriter(&stderrBuf, log.Writer(logrus.ErrorLevel, false))

	sshManager.session.Stdout = stdoutWriter
	sshManager.session.Stderr = stderrWriter
	sshManager.session.Stdin = pipes.grpcStdoutReader

	// Start the session command
	log.Debugf("run command '%s'", command)
	err = sshManager.session.Start(command)
	if err != nil {
		return nil, fmt.Errorf("start ssh command %w", err)
	}

	// Run tunnel server
	result, err := tunnelServerFunc(cancelCtx, pipes.grpcStdinWriter, pipes.grpcStdoutReader)
	if err != nil {
		return nil, fmt.Errorf("start tunnel server %w", err)
	}

	// Wait for session to complete
	sessionErr := sshManager.session.Wait()
	if sessionErr != nil {
		log.Debugf("SSH session error %v", sessionErr)
		if stderrBuf.Len() > 0 {
			log.Debugf("SSH session stderr %s", stderrBuf.String())
		}
	}

	// Wait for injection to complete
	if injErr := <-injectionDone; injErr != nil {
		return result, injErr
	}

	return result, sessionErr
}
