package ssh

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/skevetter/devpod/pkg/stdio"
	"golang.org/x/crypto/ssh"
)

func NewSSHPassClient(user, addr, password string) (*ssh.Client, error) {
	clientConfig := &ssh.ClientConfig{
		Auth:            []ssh.AuthMethod{},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	clientConfig.Auth = append(clientConfig.Auth, ssh.Password(password))

	if user != "" {
		clientConfig.User = user
	}

	client, err := ssh.Dial("tcp", addr, clientConfig)
	if err != nil {
		return nil, fmt.Errorf("dial to %v failed: %w", addr, err)
	}

	return client, nil
}

func NewSSHClient(user, addr string, keyBytes []byte) (*ssh.Client, error) {
	sshConfig, err := ConfigFromKeyBytes(keyBytes)
	if err != nil {
		return nil, err
	}

	if user != "" {
		sshConfig.User = user
	}

	client, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		return nil, fmt.Errorf("dial to %v failed: %w", addr, err)
	}

	return client, nil
}

func StdioClient(reader io.Reader, writer io.WriteCloser, exitOnClose bool) (*ssh.Client, error) {
	return StdioClientFromKeyBytesWithUser(nil, reader, writer, "", exitOnClose)
}

func StdioClientWithUser(reader io.Reader, writer io.WriteCloser, user string, exitOnClose bool) (*ssh.Client, error) {
	return StdioClientFromKeyBytesWithUser(nil, reader, writer, user, exitOnClose)
}

func StdioClientFromKeyBytesWithUser(keyBytes []byte, reader io.Reader, writer io.WriteCloser, user string, exitOnClose bool) (*ssh.Client, error) {
	conn := stdio.NewStdioStream(reader, writer, exitOnClose, 0)
	clientConfig, err := ConfigFromKeyBytes(keyBytes)
	if err != nil {
		return nil, err
	}

	clientConfig.User = user
	c, chans, req, err := ssh.NewClientConn(conn, "stdio", clientConfig)
	if err != nil {
		return nil, err
	}

	return ssh.NewClient(c, chans, req), nil
}

func ConfigFromKeyBytes(keyBytes []byte) (*ssh.ClientConfig, error) {
	clientConfig := &ssh.ClientConfig{
		Auth:            []ssh.AuthMethod{},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	// key file authentication?
	if len(keyBytes) > 0 {
		signer, err := ssh.ParsePrivateKey(keyBytes)
		if err != nil {
			return nil, fmt.Errorf("parse private key: %w", err)
		}

		clientConfig.Auth = append(clientConfig.Auth, ssh.PublicKeys(signer))
	}
	return clientConfig, nil
}

type RunOptions struct {
	Context context.Context
	Client  *ssh.Client
	Command string
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
	EnvVars map[string]string
}

// ExitError wraps an SSH exit error with the exit code.
type ExitError struct {
	ExitCode int
	Err      error
}

func (e *ExitError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("exit status %d: %v", e.ExitCode, e.Err)
	}
	return fmt.Sprintf("exit status %d", e.ExitCode)
}

func (e *ExitError) Unwrap() error {
	return e.Err
}

func (opts *RunOptions) validate() error {
	if opts.Context == nil {
		return fmt.Errorf("context is required")
	}
	if opts.Client == nil {
		return fmt.Errorf("SSH client is required")
	}
	if opts.Command == "" {
		return fmt.Errorf("command is required")
	}
	return nil
}

func Run(opts RunOptions) error {
	if err := opts.validate(); err != nil {
		return err
	}

	sess, err := opts.Client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer func() { _ = sess.Close() }()

	// Set environment variables (best effort - SSH servers may reject env vars or not support them)
	for k, v := range opts.EnvVars {
		_ = sess.Setenv(k, v) // Ignore errors - command should work without env vars
	}

	if err := setupContextCancellation(opts.Context, sess); err != nil {
		return err
	}

	sess.Stdin = opts.Stdin
	sess.Stdout = opts.Stdout
	sess.Stderr = opts.Stderr

	err = sess.Run(opts.Command)
	if err != nil {
		return handleRunError(err, opts.Command)
	}

	return nil
}

func setupContextCancellation(ctx context.Context, sess *ssh.Session) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context already cancelled: %w", err)
	}
	exit := make(chan struct{})
	go func() {
		defer close(exit)
		select {
		case <-ctx.Done():
			_ = sess.Signal(ssh.SIGINT) // Send interrupt, let defer handle close
		case <-exit:
		}
	}()
	return nil
}

func handleRunError(err error, command string) error {
	// Check for exit errors with exit codes
	var exitErr *ssh.ExitError
	if errors.As(err, &exitErr) {
		exitCode := exitErr.ExitStatus()

		// Exit codes 128+N indicate death by signal N
		// 130 = 128 + 2 (SIGINT) - Ctrl+C (user interrupted)
		// 129 = 128 + 1 (SIGHUP) - hangup (terminal closed)
		// 143 = 128 + 15 (SIGTERM) - graceful termination
		// These are "normal" ways to exit an interactive session
		if exitCode == 130 || exitCode == 129 || exitCode == 143 {
			return nil // Don't treat user interrupts as errors
		}

		// Return exit code for all other cases
		return &ExitError{
			ExitCode: exitCode,
			Err:      exitErr,
		}
	}

	// Provide context for common errors
	if errors.Is(err, io.EOF) {
		return fmt.Errorf("SSH session closed unexpectedly (EOF) while running: %s", command)
	}

	return fmt.Errorf("SSH command failed: %w (command: %s)", err, command)
}
