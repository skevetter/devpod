package config

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/skevetter/devpod/pkg/shell"
	"github.com/skevetter/log"
)

type UserEnvProbe string

const (
	LoginInteractiveShellProbe UserEnvProbe = "loginInteractiveShell"
	LoginShellProbe            UserEnvProbe = "loginShell"
	InteractiveShellProbe      UserEnvProbe = "interactiveShell"
	NoneProbe                  UserEnvProbe = "none"

	DefaultUserEnvProbe UserEnvProbe = LoginInteractiveShellProbe
)

func NewUserEnvProbe(probe string) (UserEnvProbe, error) {
	switch probe {
	case string(LoginInteractiveShellProbe):
		return LoginInteractiveShellProbe, nil
	case string(LoginShellProbe):
		return LoginShellProbe, nil
	case string(InteractiveShellProbe):
		return InteractiveShellProbe, nil
	case string(NoneProbe):
		return NoneProbe, nil
	case "":
		return DefaultUserEnvProbe, nil
	default:
		return "", fmt.Errorf("invalid userEnvProbe \"%s\", supported are \"%s\"", probe,
			strings.Join([]string{
				string(LoginInteractiveShellProbe),
				string(LoginShellProbe),
				string(InteractiveShellProbe),
				string(NoneProbe),
			}, ","))
	}
}

func ProbeUserEnv(ctx context.Context, probe string, userName string, log log.Logger) (map[string]string, error) {
	userEnvProbe, err := NewUserEnvProbe(probe)
	if err != nil {
		log.Warnf("Get user env probe: %v", err)
		log.Warnf("Falling back to default user env probe: %s", DefaultUserEnvProbe)
		userEnvProbe = DefaultUserEnvProbe
	}
	if userEnvProbe == NoneProbe {
		return map[string]string{}, nil
	}

	preferredShell, err := shell.GetShell(userName)
	if err != nil {
		return nil, fmt.Errorf("find shell for user %s: %w", userName, err)
	}

	log.Debugf("running user env probe with shell \"%s\", probe \"%s\", user \"%s\" and command \"%s\"",
		strings.Join(preferredShell, " "), string(userEnvProbe), userName, "cat /proc/self/environ")

	probedEnv, err := doProbe(ctx, userEnvProbe, preferredShell, userName, "cat /proc/self/environ", '\x00', log)
	if err != nil {
		log.Debugf("running user env probe with shell \"%s\", probe \"%s\", user \"%s\" and command \"%s\"",
			strings.Join(preferredShell, " "), string(userEnvProbe), userName, "printenv")

		newProbedEnv, newErr := doProbe(ctx, userEnvProbe, preferredShell, userName, "printenv", '\n', log)
		if newErr != nil {
			log.Warnf("failed to probe user environment variables: %v, %v", err, newErr)
		} else {
			probedEnv = newProbedEnv
		}
	}
	if probedEnv == nil {
		probedEnv = map[string]string{}
	}

	return probedEnv, nil
}

func parseProbeOutput(out []byte, sep byte, log log.Logger) map[string]string {
	// Parse NUL-separated NAME=VALUE entries robustly.
	entries := bytes.Split(out, []byte{sep})
	retEnv := make(map[string]string, len(entries))

	for _, e := range entries {
		if len(e) == 0 {
			// Skip trailing NUL or empty entry.
			continue
		}
		// Split on first '=' only.
		name, value, ok := bytes.Cut(e, []byte{'='})
		if !ok || len(name) == 0 {
			log.Debugf("failed to parse env entry: %q", string(e))
			continue
		}
		// Do NOT TrimSpace; values may intentionally start/end with whitespace/newlines.
		retEnv[string(name)] = string(value)
	}

	return retEnv
}

func doProbe(ctx context.Context, userEnvProbe UserEnvProbe, preferredShell []string, userName string, probeCmd string, sep byte, log log.Logger) (map[string]string, error) {
	args := preferredShell
	args = append(args, getShellArgs(userEnvProbe, userName, probeCmd)...)

	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(timeoutCtx, args[0], args[1:]...)

	err := PrepareCmdUser(cmd, userName)
	if err != nil {
		return nil, fmt.Errorf("prepare probe: %w", err)
	}

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("probe user env: %w", err)
	}

	retEnv := parseProbeOutput(out, sep, log)

	delete(retEnv, "PWD")

	return retEnv, nil
}

func getShellArgs(userEnvProbe UserEnvProbe, user, command string) []string {
	args := []string{}
	switch userEnvProbe {
	case LoginInteractiveShellProbe:
		args = append(args, "-lic")
	case LoginShellProbe:
		args = append(args, "-lc")
	case InteractiveShellProbe:
		args = append(args, "-ic")
	// shouldn't happen, added just for linting
	case NoneProbe:
		args = append(args, "-c")
	default:
		args = append(args, "-c")
	}
	args = append(args, command)

	return args
}
