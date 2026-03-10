package agent

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/cmd/flags"
	"github.com/skevetter/devpod/pkg/agent"
	"github.com/skevetter/devpod/pkg/client/clientimplementation"
	"github.com/skevetter/devpod/pkg/driver/custom"
	provider2 "github.com/skevetter/devpod/pkg/provider"
	"github.com/skevetter/log"
	"github.com/spf13/cobra"
)

// DaemonCmd holds the cmd flags.
type DaemonCmd struct {
	*flags.GlobalFlags

	Interval string
}

// NewDaemonCmd creates a new command.
func NewDaemonCmd(flags *flags.GlobalFlags) *cobra.Command {
	cmd := &DaemonCmd{
		GlobalFlags: flags,
	}
	daemonCmd := &cobra.Command{
		Use:   "daemon",
		Short: "Watches for activity and stops the server due to inactivity",
		Args:  cobra.NoArgs,
		RunE: func(cobraCmd *cobra.Command, _ []string) error {
			return cmd.Run(cobraCmd.Context())
		},
	}
	daemonCmd.Flags().
		StringVar(&cmd.Interval, "interval", "", "The interval how to poll workspaces")
	return daemonCmd
}

func (cmd *DaemonCmd) Run(ctx context.Context) error {
	logFolder, err := agent.GetAgentDaemonLogFolder(cmd.AgentDir)
	if err != nil {
		return err
	}

	logger := log.NewFileLogger(filepath.Join(logFolder, "agent-daemon.log"), logrus.InfoLevel)
	logger.Infof("starting DevPod daemon patrol at %s", logFolder)

	// start patrolling
	cmd.patrol(ctx, logger)

	return nil
}

func (cmd *DaemonCmd) patrol(ctx context.Context, log log.Logger) {
	// make sure we don't immediately resleep on startup
	cmd.initialTouch(log)

	// parse the daemon interval
	interval := time.Second * 60
	if cmd.Interval != "" {
		parsed, err := time.ParseDuration(cmd.Interval)
		if err == nil {
			interval = parsed
		}
	}

	// loop over workspace configs and check their last ModTime
	for {
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			cmd.doOnce(ctx, log)
		}
	}
}

func (cmd *DaemonCmd) doOnce(ctx context.Context, log log.Logger) {
	var latestActivity *time.Time
	var workspace *provider2.AgentWorkspaceInfo

	// get base folder
	baseFolder, err := agent.FindAgentHomeFolder(cmd.AgentDir)
	if err != nil {
		return
	}

	// get all workspace configs
	pattern := baseFolder + "/contexts/*/workspaces/*/" + provider2.WorkspaceConfigFile
	matches, err := filepath.Glob(pattern)
	if err != nil {
		log.Errorf("error globing pattern %s: %v", pattern, err)
		return
	}

	// check when the last touch was
	latestActivity, workspace = findLatestActivity(matches, log)

	// should we run shutdown command?
	if latestActivity == nil {
		if len(matches) == 0 {
			log.Infof("no workspaces found in path %q", baseFolder)
		} else {
			log.Infof(
				"%d workspaces found in path %q, but none of them had any auto-stop "+
					"configured or were still running / never completed",
				len(matches),
				baseFolder,
			)
		}
		return
	}

	cmd.checkAndShutdown(ctx, latestActivity, workspace, log)
}

func (cmd *DaemonCmd) checkAndShutdown(
	ctx context.Context,
	latestActivity *time.Time,
	workspace *provider2.AgentWorkspaceInfo,
	log log.Logger,
) {
	// check timeout
	timeout := agent.DefaultInactivityTimeout
	if workspace.Agent.Timeout != "" {
		var err error
		timeout, err = time.ParseDuration(workspace.Agent.Timeout)
		if err != nil {
			log.Errorf("error parsing inactivity timeout: %v", err)
			timeout = agent.DefaultInactivityTimeout
		}
	}
	if latestActivity.Add(timeout).After(time.Now()) {
		log.Infof(
			"Workspace %q has latest activity at %q, will auto-stop machine in %s",
			workspace.Workspace.ID,
			latestActivity.String(),
			time.Until(latestActivity.Add(timeout)).String(),
		)
		return
	}

	// run shutdown command
	cmd.runShutdownCommand(ctx, workspace, log)
}

func (cmd *DaemonCmd) runShutdownCommand(
	ctx context.Context,
	workspace *provider2.AgentWorkspaceInfo,
	log log.Logger,
) {
	// get environ
	environ, err := custom.ToEnvironWithBinaries(workspace, log)
	if err != nil {
		log.Errorf("%v", err)
		return
	}

	// we run the timeout command now
	buf := &bytes.Buffer{}
	log.Infof(
		"run shutdown command for workspace %s: %s",
		workspace.Workspace.ID,
		strings.Join(workspace.Agent.Exec.Shutdown, " "),
	)
	err = clientimplementation.RunCommand(clientimplementation.RunCommandOptions{
		Ctx:     ctx,
		Command: workspace.Agent.Exec.Shutdown,
		Environ: environ,
		Stdout:  buf,
		Stderr:  buf,
	})
	if err != nil {
		log.Errorf(
			"error running %s %s: %v",
			strings.Join(workspace.Agent.Exec.Shutdown, " "),
			buf.String(),
			err,
		)
		return
	}

	log.Infof("ran command: %s", buf.String())
}

func (cmd *DaemonCmd) initialTouch(log log.Logger) {
	// get base folder
	baseFolder, err := agent.FindAgentHomeFolder(cmd.AgentDir)
	if err != nil {
		return
	}

	// get workspace configs
	pattern := baseFolder + "/contexts/*/workspaces/*/" + provider2.WorkspaceConfigFile
	matches, err := filepath.Glob(pattern)
	if err != nil {
		log.Errorf("error globbing pattern %s: %v", pattern, err)
		return
	}

	// check when the last touch was
	now := time.Now()
	for _, match := range matches {
		if err := os.Chtimes(match, now, now); err != nil {
			log.Errorf("error touching workspace config %s: %v", match, err)
			continue
		}
	}
}

func findLatestActivity(
	matches []string,
	log log.Logger,
) (*time.Time, *provider2.AgentWorkspaceInfo) {
	var latestActivity *time.Time
	var workspace *provider2.AgentWorkspaceInfo
	for _, match := range matches {
		activity, activityWorkspace, err := getActivity(match, log)
		if err != nil {
			log.Errorf("error checking for inactivity: %v", err)
			continue
		} else if activity == nil {
			continue
		}

		if latestActivity == nil || activity.After(*latestActivity) {
			latestActivity = activity
			workspace = activityWorkspace
		}
	}
	return latestActivity, workspace
}

func getActivity(
	workspaceConfig string,
	log log.Logger,
) (*time.Time, *provider2.AgentWorkspaceInfo, error) {
	workspace, err := agent.ParseAgentWorkspaceInfo(workspaceConfig)
	if err != nil {
		log.Errorf("error reading %s: %v", workspaceConfig, err)
		return nil, nil, nil
	}

	// check if shutdown is configured
	if len(workspace.Agent.Exec.Shutdown) == 0 {
		return nil, nil, nil
	}

	// check last access time
	stat, err := os.Stat(workspaceConfig)
	if err != nil {
		return nil, nil, err
	}

	// check if workspace is locked
	t := stat.ModTime()
	if agent.HasWorkspaceBusyFile(filepath.Dir(workspaceConfig)) {
		t = t.Add(time.Minute * 20)
	}

	// check if timeout
	return &t, workspace, nil
}
