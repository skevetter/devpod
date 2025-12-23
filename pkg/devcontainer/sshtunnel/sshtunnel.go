package sshtunnel

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
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
		defer log.Debugf("Done executing ssh server helper command")
		defer cancel()

		writer := log.Writer(logrus.InfoLevel, false)
		defer func() { _ = writer.Close() }()

		log.Debugf("Inject and run command: %s", sshCommand)
		err := agentInject(ctx, sshCommand, sshTunnelStdinReader, sshTunnelStdoutWriter, writer)
		if err != nil && !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "signal: ") {
			errChan <- fmt.Errorf("executing agent command %w", err)
		} else {
			errChan <- nil
		}
	}()

	if addPrivateKeys {
		log.Debug("Adding ssh keys to agent, disable via 'devpod context set-options -o SSH_ADD_PRIVATE_KEYS=false'")
		err := devssh.AddPrivateKeysToAgent(ctx, log)
		if err != nil {
			log.Debugf("Error adding private keys to ssh-agent: %v", err)
		}
	}

	// create pipes
	gRPCConnStdoutReader, gRPCConnStdoutWriter, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	gRPCConnStdinReader, gRPCConnStdinWriter, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer func() { _ = gRPCConnStdoutWriter.Close() }()
	defer func() { _ = gRPCConnStdinWriter.Close() }()

	// connect to container
	go func() {
		defer cancel()

		log.Debugf("Attempting to create SSH client")
		// start ssh client as root / default user
		sshClient, err := devssh.StdioClient(sshTunnelStdoutReader, sshTunnelStdinWriter, false)
		if err != nil {
			errChan <- fmt.Errorf("create ssh client %w", err)
			return
		}
		defer log.Debugf("Connection to SSH Server closed")
		defer func() { _ = sshClient.Close() }()

		log.Debugf("SSH client created")
		log.Debugf("Waiting for SSH server to be ready")
		var sess *ssh.Session
		timeout := time.After(60 * time.Second)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-timeout:
				errChan <- errors.New("SSH server not ready after 60 seconds")
				return
			case <-cancelCtx.Done():
				errChan <- cancelCtx.Err()
				return
			case <-ticker.C:
				var err error
				sess, err = sshClient.NewSession()
				if err == nil {
					log.Debugf("SSH session created")
					goto sessionReady
				}
				log.Debugf("SSH server not ready %v", err)
			}
		}

	sessionReady:
		defer func() { _ = sess.Close() }()

		identityAgent := devsshagent.GetSSHAuthSocket()
		if identityAgent != "" {
			log.Debugf("Forwarding ssh-agent using %s", identityAgent)
			err = devsshagent.ForwardToRemote(sshClient, identityAgent)
			if err != nil {
				errChan <- fmt.Errorf("forward agent %w", err)
			}
			err = devsshagent.RequestAgentForwarding(sess)
			if err != nil {
				errChan <- fmt.Errorf("request agent forwarding failed %w", err)
			}
		}

		stdoutWriter := log.Writer(logrus.InfoLevel, false)
		defer func() { _ = stdoutWriter.Close() }()

		var stderrBuf bytes.Buffer
		stderrWriter := io.MultiWriter(&stderrBuf, log.Writer(logrus.ErrorLevel, false))

		err = devssh.Run(ctx, sshClient, command, gRPCConnStdinReader, gRPCConnStdoutWriter, stderrWriter, nil)
		if err != nil {
			if stderrBuf.Len() > 0 {
				errChan <- fmt.Errorf("run agent command %w\n%s", err, stderrBuf.String())
			} else {
				errChan <- fmt.Errorf("run agent command %w", err)
			}
		} else {
			errChan <- nil
		}
	}()

	result, err := tunnelServerFunc(cancelCtx, gRPCConnStdinWriter, gRPCConnStdoutReader)
	if err != nil {
		return nil, fmt.Errorf("start tunnel server %w", err)
	}

	// wait until command finished
	if err := <-errChan; err != nil {
		return result, err
	}

	return result, <-errChan
}
