package container

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/pkg/agent"
	agentd "github.com/skevetter/devpod/pkg/daemon/agent"
	"github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/skevetter/devpod/pkg/platform/client"
	"github.com/skevetter/devpod/pkg/ts"
	"github.com/skevetter/log"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

const (
	RootDir          = "/var/devpod"
	DaemonConfigPath = "/var/run/secrets/devpod/daemon_config"
)

type DaemonCmd struct {
	Config *agentd.DaemonConfig
	Log    log.Logger
}

// NewDaemonCmd creates the merged daemon command.
func NewDaemonCmd() *cobra.Command {
	cmd := &DaemonCmd{
		Config: &agentd.DaemonConfig{},
		Log:    log.NewStreamLogger(os.Stdout, os.Stderr, logrus.InfoLevel),
	}
	daemonCmd := &cobra.Command{
		Use:   "daemon",
		Short: "Starts the DevPod network daemon, SSH server and monitors container activity if timeout is set",
		Args:  cobra.NoArgs,
		RunE:  cmd.Run,
	}
	daemonCmd.Flags().
		StringVar(&cmd.Config.Timeout, "timeout", "", "The timeout to stop the container after")
	return daemonCmd
}

func (cmd *DaemonCmd) Run(c *cobra.Command, args []string) error {
	if err := cmd.loadConfig(); err != nil {
		return err
	}

	var timeoutDuration time.Duration
	if cmd.Config.Timeout != "" {
		var err error
		timeoutDuration, err = time.ParseDuration(cmd.Config.Timeout)
		if err != nil {
			return fmt.Errorf("failed to parse timeout duration: %w", err)
		}
		if timeoutDuration > 0 {
			if err := os.WriteFile(
				agent.ContainerActivityFile,
				nil,
				0o666,
			); err != nil { // #nosec G306
				return fmt.Errorf("failed to create activity file: %w", err)
			}
			if err := os.Chmod(agent.ContainerActivityFile, 0o666); err != nil { // #nosec G302
				return fmt.Errorf("failed to set activity file permissions: %w", err)
			}
		}
	}

	ctx, stop := signal.NotifyContext(c.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	g, ctx := errgroup.WithContext(ctx)
	var tasksStarted bool

	// Start process reaper.
	if os.Getpid() == 1 {
		g.Go(func() error {
			agentd.RunProcessReaper()
			<-ctx.Done()
			return nil
		})
	}

	// Start Tailscale networking server.
	if cmd.shouldRunNetworkServer() {
		tasksStarted = true
		g.Go(func() error {
			return runNetworkServer(ctx, cmd)
		})
	}

	// Start timeout monitor.
	if timeoutDuration > 0 {
		tasksStarted = true
		g.Go(func() error {
			return runTimeoutMonitor(ctx, timeoutDuration)
		})
	}

	// Start ssh server.
	if cmd.shouldRunSsh() {
		tasksStarted = true
		g.Go(func() error {
			return runSshServer(ctx, cmd)
		})
	}

	// In case no task is configured, just wait indefinitely.
	if !tasksStarted {
		g.Go(func() error {
			<-ctx.Done()
			return nil
		})
	}

	err := g.Wait()
	if err != nil {
		cmd.Log.Errorf("daemon error: %v", err)
		os.Exit(1)
	}
	os.Exit(0)
	return nil // Unreachable but needed.
}

// loadConfig loads the daemon configuration from base64-encoded JSON.
// If a CLI-provided timeout exists, it will override the timeout in the config.
func (cmd *DaemonCmd) loadConfig() error {
	// check local file
	encodedCfg := ""
	configBytes, err := os.ReadFile(DaemonConfigPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// check environment variable
			encodedCfg = os.Getenv(config.WorkspaceDaemonConfigExtraEnvVar)
		} else {
			return fmt.Errorf("get daemon config file %s: %w", DaemonConfigPath, err)
		}
	} else {
		encodedCfg = string(configBytes)
	}

	if strings.TrimSpace(encodedCfg) != "" {
		decoded, err := base64.StdEncoding.DecodeString(encodedCfg)
		if err != nil {
			return fmt.Errorf("error decoding daemon config: %w", err)
		}
		var cfg agentd.DaemonConfig
		if err = json.Unmarshal(decoded, &cfg); err != nil {
			return fmt.Errorf("error unmarshalling daemon config: %w", err)
		}
		if cmd.Config.Timeout != "" {
			cfg.Timeout = cmd.Config.Timeout
		}
		cmd.Config = &cfg
	}

	return nil
}

// shouldRunNetworkServer returns true if the required platform parameters are present.
func (cmd *DaemonCmd) shouldRunNetworkServer() bool {
	return cmd.Config.Platform.AccessKey != "" &&
		cmd.Config.Platform.PlatformHost != "" &&
		cmd.Config.Platform.WorkspaceHost != ""
}

// shouldRunSsh returns true if at least one SSH configuration value is provided.
func (cmd *DaemonCmd) shouldRunSsh() bool {
	return cmd.Config.Ssh.Workdir != "" || cmd.Config.Ssh.User != ""
}

// runTimeoutMonitor monitors the activity file and signals an error if the timeout is exceeded.
func runTimeoutMonitor(
	ctx context.Context,
	duration time.Duration,
) error {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			stat, err := os.Stat(agent.ContainerActivityFile)
			if err != nil {
				continue
			}
			if !stat.ModTime().Add(duration).After(time.Now()) {
				return errors.New("timeout reached, terminating daemon")
			}
		}
	}
}

// runNetworkServer starts the network server.
func runNetworkServer(
	ctx context.Context,
	cmd *DaemonCmd,
) error {
	if err := os.MkdirAll(RootDir, os.ModePerm); err != nil {
		return err
	}
	logger := initLogging()
	config := client.NewConfig()
	config.AccessKey = cmd.Config.Platform.AccessKey
	config.Host = "https://" + cmd.Config.Platform.PlatformHost
	config.Insecure = true
	baseClient := client.NewClientFromConfig(config)
	if err := baseClient.RefreshSelf(ctx); err != nil {
		return fmt.Errorf("failed to refresh client: %w", err)
	}
	tsServer := ts.NewWorkspaceServer(&ts.WorkspaceServerConfig{
		AccessKey:     cmd.Config.Platform.AccessKey,
		PlatformHost:  ts.RemoveProtocol(cmd.Config.Platform.PlatformHost),
		WorkspaceHost: cmd.Config.Platform.WorkspaceHost,
		Client:        baseClient,
		RootDir:       RootDir,
		LogF: func(format string, args ...any) {
			logger.Infof(format, args...)
		},
	}, logger)
	if err := tsServer.Start(ctx); err != nil {
		return fmt.Errorf("network server: %w", err)
	}
	return nil
}

// runSshServer starts the SSH server, sending SIGTERM on context cancellation
// with a grace period before force-killing.
func runSshServer(ctx context.Context, cmd *DaemonCmd) error {
	binaryPath, err := os.Executable()
	if err != nil {
		return err
	}

	args := []string{"agent", "container", "ssh-server"}
	if cmd.Config.Ssh.Workdir != "" {
		args = append(args, "--workdir", cmd.Config.Ssh.Workdir)
	}
	if cmd.Config.Ssh.User != "" {
		args = append(args, "--remote-user", cmd.Config.Ssh.User)
	}

	sshCmd := exec.CommandContext(ctx, binaryPath, args...) // #nosec G204
	sshCmd.Stdout = os.Stdout
	sshCmd.Stderr = os.Stderr
	sshCmd.Cancel = func() error {
		return sshCmd.Process.Signal(syscall.SIGTERM)
	}
	sshCmd.WaitDelay = 5 * time.Second // Graceful shutdown before force-killing.

	if err := sshCmd.Run(); err != nil && ctx.Err() == nil {
		return fmt.Errorf("SSH server exited abnormally: %w", err)
	}
	return nil
}

// initLogging initializes logging and returns a combined logger.
func initLogging() log.Logger {
	return log.NewStdoutLogger(nil, os.Stdout, os.Stderr, logrus.InfoLevel)
}
