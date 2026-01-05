package shell

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strings"
	"time"

	"github.com/pkg/errors"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

func RunEmulatedShell(ctx context.Context, command string, stdin io.Reader, stdout io.Writer, stderr io.Writer, env []string) error {
	command = strings.ReplaceAll(command, "\r", "")

	// Let's parse the complete command
	parsed, err := syntax.NewParser().Parse(strings.NewReader(command), "")
	if err != nil {
		return fmt.Errorf("parse shell command %w", err)
	}

	// use system default as environ if unspecified
	if env == nil {
		env = []string{}
		env = append(env, os.Environ()...)
	}

	// Get current working directory
	dir, err := os.Getwd()
	if err != nil {
		return err
	}

	// create options
	defaultOpenHandler := interp.DefaultOpenHandler()
	defaultExecHandler := interp.DefaultExecHandler(2 * time.Second)
	options := []interp.RunnerOption{
		interp.StdIO(stdin, stdout, stderr),
		interp.Env(expand.ListEnviron(env...)),
		interp.Dir(dir),
		interp.ExecHandlers(func(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
			return func(ctx context.Context, args []string) error {
				return defaultExecHandler(ctx, args)
			}
		}),
		interp.OpenHandler(func(ctx context.Context, path string, flag int, perm os.FileMode) (io.ReadWriteCloser, error) {
			if path == "/dev/null" {
				return devNull{}, nil
			}

			return defaultOpenHandler(ctx, path, flag, perm)
		}),
	}

	// Create shell runner
	r, err := interp.New(options...)
	if err != nil {
		return fmt.Errorf("create shell runner %w", err)
	}

	// Run command
	err = r.Run(ctx, parsed)
	if err != nil {
		var exitStatus interp.ExitStatus
		if errors.As(err, &exitStatus) && exitStatus == 0 {
			return nil
		}

		return err
	}

	return nil
}

var _ io.ReadWriteCloser = devNull{}

type devNull struct{}

func (devNull) Read(_ []byte) (int, error) {
	return 0, io.EOF
}

func (devNull) Write(p []byte) (int, error) {
	return len(p), nil
}

func (devNull) Close() error {
	return nil
}

func GetShell(userName string) ([]string, error) {
	// try to get a shell
	if runtime.GOOS != "windows" {
		// Try shells in order of preference, validating each one
		shellCandidates := []string{}

		// First try to get login shell from getent
		if shell, err := getUserShell(userName); err == nil {
			shellCandidates = append(shellCandidates, shell)
		}

		// Add $SHELL env var
		if shell, ok := os.LookupEnv("SHELL"); ok {
			shellCandidates = append(shellCandidates, shell)
		}

		// Add common shell locations
		shellCandidates = append(shellCandidates, "/bin/bash", "/usr/bin/bash", "/bin/sh", "/usr/bin/sh")

		// Test each candidate
		for _, shell := range shellCandidates {
			if isExecutableShell(shell) {
				return []string{shell}, nil
			}
		}

		// Try PATH-based discovery
		for _, shellName := range []string{"bash", "sh"} {
			if shellPath, err := exec.LookPath(shellName); err == nil && isExecutableShell(shellPath) {
				return []string{shellPath}, nil
			}
		}
	}

	// fallback to our in-built shell
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}

	return []string{executable, "helper", "sh"}, nil
}

// isExecutableShell checks if a shell path exists and is executable
func isExecutableShell(shellPath string) bool {
	if shellPath == "" {
		return false
	}

	info, err := os.Stat(shellPath)
	if err != nil {
		return false
	}

	// Check if it's a regular file and executable
	return info.Mode().IsRegular() && (info.Mode().Perm()&0111) != 0
}

func getUserShell(userName string) (string, error) {
	currentUser, err := findUser(userName)
	if err != nil {
		return "", err
	}
	output, err := exec.Command("getent", "passwd", currentUser.Username).Output()
	if err != nil {
		return "", err
	}

	shell := strings.Split(string(output), ":")
	if len(shell) != 7 {
		return "", fmt.Errorf("unexpected getent format: %s", string(output))
	}

	loginShell := strings.TrimSpace(shell[6])
	if loginShell == "nologin" || loginShell == "/usr/sbin/nologin" || loginShell == "/sbin/nologin" {
		return "", fmt.Errorf("no login shell configured")
	}

	// Return the full path, not just the basename
	return loginShell, nil
}

func findUser(userName string) (*user.User, error) {
	if userName != "" {
		u, err := user.Lookup(userName)
		if err != nil {
			return nil, err
		}
		return u, nil
	}

	return user.Current()
}
