package workspace

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/loft-sh/log"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/spf13/cobra"
)

const (
	RootDir                          = "/var/devpod"
	DaemonConfigPath                 = "/var/run/secrets/devpod/daemon_config"
	WorkspaceDaemonConfigExtraEnvVar = "DEVPOD_WORKSPACE_DAEMON_CONFIG"
)

// Daemon manages workspace services
type Daemon struct {
	Config *DaemonConfig
	log    log.Logger
}

// NewDaemon creates a new workspace daemon
func NewDaemon() *Daemon {
	return &Daemon{
		Config: &DaemonConfig{},
		log:    log.NewStreamLogger(os.Stdout, os.Stderr, logrus.InfoLevel),
	}
}

// Run starts daemon subsystems
func (d *Daemon) Run(c *cobra.Command, args []string) error {
	ctx := c.Context()
	errChan := make(chan error, 4)
	var wg sync.WaitGroup

	if err := d.loadConfig(); err != nil {
		return err
	}

	// Parse timeout
	var timeoutDuration time.Duration
	if d.Config.Timeout != "" {
		var err error
		timeoutDuration, err = time.ParseDuration(d.Config.Timeout)
		if err != nil {
			return errors.Wrap(err, "parse timeout")
		}
		if timeoutDuration > 0 {
			if err := SetupActivityFile(); err != nil {
				return err
			}
		}
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	tasksStarted := false

	// Process reaper if PID 1
	if os.Getpid() == 1 {
		wg.Add(1)
		go RunProcessReaper()
	}

	// Network server
	if d.shouldRunNetworkServer() {
		tasksStarted = true
		wg.Add(1)
		go RunNetworkServer(ctx, d, errChan, &wg, RootDir)
	}

	// Timeout monitor
	if timeoutDuration > 0 {
		tasksStarted = true
		wg.Add(1)
		go RunTimeoutMonitor(ctx, timeoutDuration, errChan, &wg, d.log)
	}

	// SSH server
	if d.shouldRunSsh() {
		tasksStarted = true
		wg.Add(1)
		go RunSshServer(ctx, d, errChan, &wg)
	}

	// Wait indefinitely if no tasks
	if !tasksStarted {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-ctx.Done()
		}()
	}

	// Handle signals
	go HandleSignals(ctx, errChan)

	// Wait for error or signal
	err := <-errChan
	cancel()
	wg.Wait()

	if err != nil {
		d.log.Errorf("Daemon error: %v", err)
		os.Exit(1)
	}
	os.Exit(0)
	return nil
}

func (d *Daemon) loadConfig() error {
	encodedCfg := ""
	configBytes, err := os.ReadFile(DaemonConfigPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			encodedCfg = os.Getenv(config.WorkspaceDaemonConfigExtraEnvVar)
		} else {
			return fmt.Errorf("read config: %w", err)
		}
	} else {
		encodedCfg = string(configBytes)
	}

	if strings.TrimSpace(encodedCfg) != "" {
		decoded, err := base64.StdEncoding.DecodeString(encodedCfg)
		if err != nil {
			return fmt.Errorf("decode config: %w", err)
		}
		var cfg DaemonConfig
		if err = json.Unmarshal(decoded, &cfg); err != nil {
			return fmt.Errorf("unmarshal config: %w", err)
		}
		if d.Config.Timeout != "" {
			cfg.Timeout = d.Config.Timeout
		}
		d.Config = &cfg
	}

	return nil
}

func (d *Daemon) shouldRunNetworkServer() bool {
	return d.Config.Platform.AccessKey != "" &&
		d.Config.Platform.PlatformHost != "" &&
		d.Config.Platform.WorkspaceHost != ""
}

func (d *Daemon) shouldRunSsh() bool {
	return d.Config.Ssh.Workdir != "" || d.Config.Ssh.User != ""
}

// HandleSignals listens for termination signals
func HandleSignals(ctx context.Context, errChan chan<- error) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	select {
	case sig := <-sigChan:
		errChan <- fmt.Errorf("signal: %v", sig)
	case <-ctx.Done():
	}
}
