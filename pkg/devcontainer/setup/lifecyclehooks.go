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
	"regexp"
	"slices"
	"strings"
	"sync"

	"al.essio.dev/pkg/shellescape"
	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/skevetter/devpod/pkg/types"
	"github.com/skevetter/log"
)

// lifecycleEnv holds the resolved environment for running lifecycle hooks.
type lifecycleEnv struct {
	remoteUser      string
	workspaceFolder string
	remoteEnv       map[string]string
}

func resolveLifecycleEnv(
	ctx context.Context,
	setupInfo *config.Result,
	log log.Logger,
) lifecycleEnv {
	mergedConfig := setupInfo.MergedConfig
	remoteUser := config.GetRemoteUser(setupInfo)
	probedEnv, err := config.ProbeUserEnv(ctx, mergedConfig.UserEnvProbe, remoteUser, log)
	if err != nil {
		log.Errorf(
			"failed to probe environment, this might lead to an incomplete setup of your workspace: error=%v",
			err,
		)
	}

	return lifecycleEnv{
		remoteUser:      remoteUser,
		workspaceFolder: setupInfo.SubstitutionContext.ContainerWorkspaceFolder,
		remoteEnv:       mergeRemoteEnv(mergedConfig.RemoteEnv, probedEnv, remoteUser),
	}
}

// RunPreAttachHooks runs lifecycle hooks up to and including postStartCommand.
// These must complete before the IDE can be opened.
func RunPreAttachHooks(ctx context.Context, setupInfo *config.Result, log log.Logger) error {
	env := resolveLifecycleEnv(ctx, setupInfo, log)
	containerDetails := setupInfo.ContainerDetails
	mergedConfig := setupInfo.MergedConfig

	// only run once per container run
	if err := run(mergedConfig.OnCreateCommands, env.remoteUser, env.workspaceFolder, env.remoteEnv,
		"onCreateCommands", containerDetails.Created, log); err != nil {
		return err
	}

	// TODO: rerun when contents changed
	if err := run(
		mergedConfig.UpdateContentCommands,
		env.remoteUser,
		env.workspaceFolder,
		env.remoteEnv,
		"updateContentCommands",
		containerDetails.Created,
		log,
	); err != nil {
		return err
	}

	// only run once per container run
	if err := run(
		mergedConfig.PostCreateCommands,
		env.remoteUser,
		env.workspaceFolder,
		env.remoteEnv,
		"postCreateCommands",
		containerDetails.Created,
		log,
	); err != nil {
		return err
	}

	// run when the container was restarted
	if err := run(
		mergedConfig.PostStartCommands,
		env.remoteUser,
		env.workspaceFolder,
		env.remoteEnv,
		"postStartCommands",
		containerDetails.State.StartedAt,
		log,
	); err != nil {
		return err
	}

	return nil
}

// RunPostAttachHooks runs postAttachCommand only.
// These run after the IDE has been opened and can be long-running.
func RunPostAttachHooks(ctx context.Context, setupInfo *config.Result, log log.Logger) error {
	env := resolveLifecycleEnv(ctx, setupInfo, log)

	// run always when attaching to the container
	return run(
		setupInfo.MergedConfig.PostAttachCommands,
		env.remoteUser,
		env.workspaceFolder,
		env.remoteEnv,
		"postAttachCommands",
		"",
		log,
	)
}

func run(
	commands []types.LifecycleHook,
	remoteUser, dir string,
	remoteEnv map[string]string,
	name, content string,
	log log.Logger,
) error {
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
			log.Infof("running %s lifecycle hook: %s %s", name, k, strings.Join(c, " "))
			currentUser, err := user.Current()
			if err != nil {
				return err
			}

			if len(c) == 0 {
				log.Debugf("skipping empty command for lifecycle hook %s", name)
				continue
			}
			args := buildCommandArgs(c, remoteUser, currentUser.Username)

			// create command
			cmd := exec.Command(args[0], args[1:]...)
			cmd.Dir = dir
			cmd.Env = os.Environ()
			cmd.Env = append(cmd.Env, remoteEnvArr...)

			// Create pipes for stdout and stderr
			stdoutPipe, err := cmd.StdoutPipe()
			if err != nil {
				return fmt.Errorf("failed to get stdout pipe: %w", err)
			}
			stderrPipe, err := cmd.StderrPipe()
			if err != nil {
				return fmt.Errorf("failed to get stderr pipe: %w", err)
			}

			// Start the command
			if err := cmd.Start(); err != nil {
				return fmt.Errorf("failed to start command: %w", err)
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
				log.Debugf(
					"failed running %s lifecycle script: command=%v, error=%v",
					k,
					cmd.Args,
					err,
				)
				return fmt.Errorf("failed to run: %s, error: %w", strings.Join(c, " "), err)
			}

			log.Donef("ran command: command=%s, args=%s", k, strings.Join(c, " "))
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
		log.Errorf("error reading pipe: error=%v", err)
	}
}

// containsError defines what log line treated as error log should contain.
func containsError(line string) bool {
	return strings.Contains(strings.ToLower(line), "error")
}

func mergeRemoteEnv(
	remoteEnv map[string]string,
	probedEnv map[string]string,
	remoteUser string,
) map[string]string {
	retEnv := map[string]string{}

	// Order matters here
	// remoteEnv should always override probedEnv as it has been specified explicitly by the devcontainer author
	maps.Copy(retEnv, probedEnv)
	maps.Copy(retEnv, remoteEnv)
	probedPath, probeOk := probedEnv["PATH"]
	remotePath, remoteOk := remoteEnv["PATH"]
	if probeOk && remoteOk {
		// merge probed PATH and remote PATH
		sbinRegex := regexp.MustCompile(`/sbin(/|$)`)
		probedTokens := strings.Split(probedPath, ":")
		insertAt := 0
		for e := range strings.SplitSeq(remotePath, ":") {
			// check if remotePath entry is in probed tokens
			i := slices.Index(probedTokens, e)
			if i == -1 {
				// only include /sbin paths for root users
				if remoteUser == "root" || !sbinRegex.MatchString(e) {
					probedTokens = slices.Insert(probedTokens, insertAt, e)
				}
			} else {
				insertAt = i + 1
			}
		}

		retEnv["PATH"] = strings.Join(probedTokens, ":")
	}

	return retEnv
}

func buildCommandArgs(c []string, remoteUser, currentUsername string) []string {
	if len(c) == 1 {
		if remoteUser != currentUsername {
			return []string{"su", remoteUser, "-c", c[0]}
		}
		return []string{"sh", "-c", c[0]}
	}
	if remoteUser != currentUsername {
		return []string{"su", remoteUser, "-c", shellescape.QuoteCommand(c)}
	}
	return c
}
