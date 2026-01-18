package setup

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/pkg/command"
	"github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/skevetter/devpod/pkg/types"
	"github.com/skevetter/log"
)

// RunLifecycleHooks executes the lifecycle commands for a development container.
func RunLifecycleHooks(ctx context.Context, setupInfo *config.Result, log log.Logger) error {
	mergedConfig := setupInfo.MergedConfig
	remoteUser := config.GetRemoteUser(setupInfo)
	probedEnv, err := config.ProbeUserEnv(ctx, mergedConfig.UserEnvProbe, remoteUser, log)
	if err != nil {
		log.WithFields(logrus.Fields{"error": err}).Error("failed to probe environment, this might lead to an incomplete setup of your workspace")
	}
	remoteEnv := mergeRemoteEnv(mergedConfig.RemoteEnv, probedEnv)

	workspaceFolder := setupInfo.SubstitutionContext.ContainerWorkspaceFolder
	containerDetails := setupInfo.ContainerDetails

	// only run once per container run
	err = run(mergedConfig.OnCreateCommands, remoteUser, workspaceFolder, remoteEnv,
		"onCreateCommands", containerDetails.Created, log)
	if err != nil {
		return err
	}

	// TODO: rerun when contents changed
	err = run(mergedConfig.UpdateContentCommands, remoteUser, workspaceFolder, remoteEnv,
		"updateContentCommands", containerDetails.Created, log)
	if err != nil {
		return err
	}

	// only run once per container run
	err = run(mergedConfig.PostCreateCommands, remoteUser, workspaceFolder, remoteEnv,
		"postCreateCommands", containerDetails.Created, log)
	if err != nil {
		return err
	}

	// run when the container was restarted
	err = run(mergedConfig.PostStartCommands, remoteUser, workspaceFolder, remoteEnv,
		"postStartCommands", containerDetails.State.StartedAt, log)
	if err != nil {
		return err
	}

	// run always when attaching to the container
	err = run(mergedConfig.PostAttachCommands, remoteUser, workspaceFolder, remoteEnv,
		"postAttachCommands", "", log)
	if err != nil {
		return err
	}

	return nil
}

func run(commands []types.LifecycleHook, remoteUser, dir string, remoteEnv map[string]string, name, content string, log log.Logger) error {
	if len(commands) == 0 {
		return nil
	}

	// check marker file
	if content != "" {
		exists, err := markerFileExists(name, content)
		if err != nil {
			return err
		} else if exists {
			return nil
		}
	}

	remoteEnvArr := []string{}
	for k, v := range remoteEnv {
		remoteEnvArr = append(remoteEnvArr, k+"="+v)
	}

	for _, cmd := range commands {
		if len(cmd) == 0 {
			continue
		}

		for k, c := range cmd {
			log.WithFields(logrus.Fields{"command": k, "args": strings.Join(c, " ")}).Info("lifecycle hook run command")
			currentUser, err := user.Current()
			if err != nil {
				return err
			}
			args := []string{}
			if remoteUser != currentUser.Username {
				args = append(args, "su", remoteUser, "-c", command.Quote(c))
			} else {
				args = append(args, "sh", "-c", command.Quote(c))
			}

			// create command
			cmd := exec.Command(args[0], args[1:]...)
			cmd.Dir = dir
			cmd.Env = os.Environ()
			cmd.Env = append(cmd.Env, remoteEnvArr...)

			// Create pipes for stdout and stderr
			stdoutPipe, err := cmd.StdoutPipe()
			if err != nil {
				return fmt.Errorf("failed to get stdout pipe %w", err)
			}
			stderrPipe, err := cmd.StderrPipe()
			if err != nil {
				return fmt.Errorf("failed to get stderr pipe %w", err)
			}

			// Start the command
			if err := cmd.Start(); err != nil {
				return fmt.Errorf("failed to start command %w", err)
			}

			// Use WaitGroup to wait for both stdout and stderr processing
			var wg sync.WaitGroup
			wg.Add(2)

			go func() {
				defer wg.Done()
				logPipeOutput(log, stdoutPipe, logrus.InfoLevel)
			}()

			go func() {
				defer wg.Done()
				logPipeOutput(log, stderrPipe, logrus.ErrorLevel)
			}()

			// Wait for command to finish
			wg.Wait()
			err = cmd.Wait()
			if err != nil {
				log.WithFields(logrus.Fields{"command": cmd.Args, "error": err}).Debug("failed running postCreateCommand lifecycle script")
				return fmt.Errorf("failed to run: %s, error %w", strings.Join(c, " "), err)
			}

			log.WithFields(logrus.Fields{"command": k, "args": strings.Join(c, " ")}).Done("ran command")
		}
	}

	return nil
}

func logPipeOutput(log log.Logger, pipe io.ReadCloser, level logrus.Level) {
	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		line := scanner.Text()
		switch level {
		case logrus.InfoLevel:
			log.Info(line)
		case logrus.ErrorLevel:
			if containsError(line) {
				log.Error(line)
			} else {
				log.Warn(line)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		log.WithFields(logrus.Fields{"error": err}).Error("error reading pipe")
	}
}

// containsError defines what log line treated as error log should contain.
func containsError(line string) bool {
	return strings.Contains(strings.ToLower(line), "error")
}

// mergeRemoteEnv merges remoteEnv with probedEnv according to the devcontainer specification.
//
// From the spec (https://containers.dev/implementors/spec/#merge-logic):
// "remoteEnv: Object of strings. Per variable, last value wins."
//
// This means:
// 1. Start with probedEnv (environment variables probed from the container)
// 2. Override with remoteEnv (explicitly set in devcontainer.json)
// 3. For each variable in remoteEnv, the remoteEnv value completely replaces any probed value
//
// remoteEnv should be used for PATH modifications:
// "If you want to reference an existing container variable while setting this one
// (like updating the PATH), use remoteEnv instead."
// Reference: https://containers.dev/implementors/json_reference/#general-properties
//
// When PATH is set in remoteEnv, it is used exactly as specified without merging
// with the probed PATH.
//
// Example from spec:
//
//	"remoteEnv": { "PATH": "${containerEnv:PATH}:/some/other/path" }
//
// Parameters:
//   - remoteEnv: Environment variables from devcontainer.json remoteEnv property
//   - probedEnv: Environment variables probed from the container using userEnvProbe
//
// Returns:
//   - Merged environment map where remoteEnv values override probedEnv values
func mergeRemoteEnv(remoteEnv map[string]string, probedEnv map[string]string) map[string]string {
	retEnv := map[string]string{}
	maps.Copy(retEnv, probedEnv)
	maps.Copy(retEnv, remoteEnv)
	return retEnv
}
