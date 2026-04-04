package agent

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/loft-sh/api/v4/pkg/devpod"
	"github.com/skevetter/devpod/pkg/command"
	pkgconfig "github.com/skevetter/devpod/pkg/config"
	"github.com/skevetter/devpod/pkg/devcontainer/config"
	provider2 "github.com/skevetter/devpod/pkg/provider"
	"github.com/skevetter/log"
)

type SshConfig struct {
	Workdir string `json:"workdir,omitempty"`
	User    string `json:"user,omitempty"`
}

type DaemonConfig struct {
	Platform devpod.PlatformOptions `json:"platform"`
	Ssh      SshConfig              `json:"ssh"`
	Timeout  string                 `json:"timeout"`
}

func BuildWorkspaceDaemonConfig(
	platformOptions devpod.PlatformOptions,
	workspaceConfig *provider2.Workspace,
	substitutionContext *config.SubstitutionContext,
	mergedConfig *config.MergedDevContainerConfig,
) (*DaemonConfig, error) {
	var workdir string
	if workspaceConfig.Source.GitSubPath != "" {
		substitutionContext.ContainerWorkspaceFolder = filepath.Join(
			substitutionContext.ContainerWorkspaceFolder,
			workspaceConfig.Source.GitSubPath,
		)
		workdir = substitutionContext.ContainerWorkspaceFolder
	}
	if workdir == "" && mergedConfig != nil {
		workdir = mergedConfig.WorkspaceFolder
	}
	if workdir == "" && substitutionContext != nil {
		workdir = substitutionContext.ContainerWorkspaceFolder
	}

	// Get remote user; default to "root" if empty.
	user := mergedConfig.RemoteUser
	if user == "" {
		user = "root"
	}

	// build info isn't required in the workspace and can be omitted
	platformOptions.Build = nil

	daemonConfig := &DaemonConfig{
		Platform: platformOptions,
		Ssh: SshConfig{
			Workdir: workdir,
			User:    user,
		},
	}

	return daemonConfig, nil
}

func GetEncodedWorkspaceDaemonConfig(
	platformOptions devpod.PlatformOptions,
	workspaceConfig *provider2.Workspace,
	substitutionContext *config.SubstitutionContext,
	mergedConfig *config.MergedDevContainerConfig,
) (string, error) {
	daemonConfig, err := BuildWorkspaceDaemonConfig(
		platformOptions,
		workspaceConfig,
		substitutionContext,
		mergedConfig,
	)
	if err != nil {
		return "", err
	}

	data, err := json.Marshal(daemonConfig)
	if err != nil {
		return "", err
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	return encoded, nil
}

const systemdDir = "/etc/systemd/system"

func serviceName() string {
	return pkgconfig.BinaryName + ".service"
}

func serviceFilePath() string {
	return filepath.Join(systemdDir, serviceName())
}

func systemdUnitContents(execStart string) string {
	return fmt.Sprintf(`[Unit]
Description=%s
After=network.target

[Service]
ExecStart=%s
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
`, pkgconfig.DaemonServiceDescription, execStart)
}

func isServiceInstalled() bool {
	_, err := os.Stat(serviceFilePath())
	return err == nil
}

func isServiceRunning() bool {
	//nolint:gosec // BinaryName is a compile-time constant, not tainted input
	out, err := exec.Command("systemctl", "is-active", pkgconfig.BinaryName).CombinedOutput()
	return err == nil && strings.TrimSpace(string(out)) == "active"
}

func InstallDaemon(agentDir string, interval string, log log.Logger) error {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return fmt.Errorf("unsupported daemon os")
	}

	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	// install ourselves with devpod agent daemon
	args := []string{executable, "agent", "daemon"}
	if agentDir != "" {
		args = append(args, "--agent-dir", agentDir)
	}
	if interval != "" {
		args = append(args, "--interval", interval)
	}

	if !isServiceInstalled() {
		unitContent := systemdUnitContents(strings.Join(args, " "))
		//nolint:gosec // systemd unit files must be world-readable (0644)
		if err := os.WriteFile(serviceFilePath(), []byte(unitContent), 0o644); err != nil {
			return fmt.Errorf("write service file: %w", err)
		}

		if out, err := exec.Command("systemctl", "daemon-reload").CombinedOutput(); err != nil {
			return fmt.Errorf("systemctl daemon-reload: %s: %w", string(out), err)
		}
	}

	// Always enable so the service starts on boot, even if it was previously disabled.
	//nolint:gosec // BinaryName is a compile-time constant, not tainted input
	if out, err := exec.Command("systemctl", "enable", pkgconfig.BinaryName).
		CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl enable: %s: %w", string(out), err)
	}

	// make sure daemon is started
	if !isServiceRunning() {
		//nolint:gosec // BinaryName is a compile-time constant, not tainted input
		if out, err := exec.Command("systemctl", "start", pkgconfig.BinaryName).
			CombinedOutput(); err != nil {
			log.Warnf("Error starting service via systemctl: %s: %v", string(out), err)

			daemonArgs := args[1:] // strip executable path
			err = command.StartBackgroundOnce("devpod.daemon", func() (*exec.Cmd, error) {
				log.Infof("started DevPod daemon into server")
				return exec.Command(
					executable,
					daemonArgs...), nil //nolint:gosec // executable is from os.Executable()
			})
			if err != nil {
				return fmt.Errorf("start daemon: %w", err)
			}
		} else {
			log.Infof("installed DevPod daemon into server")
		}
	}

	return nil
}

func RemoveDaemon() error {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return fmt.Errorf("unsupported daemon os")
	}

	if !isServiceInstalled() {
		return nil
	}

	// stop and disable the service
	//nolint:gosec // BinaryName is a compile-time constant
	_ = exec.Command("systemctl", "stop", pkgconfig.BinaryName).Run()
	//nolint:gosec // BinaryName is a compile-time constant
	_ = exec.Command("systemctl", "disable", pkgconfig.BinaryName).Run()

	if err := os.Remove(serviceFilePath()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove service file: %w", err)
	}

	if out, err := exec.Command("systemctl", "daemon-reload").CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %s: %w", string(out), err)
	}

	return nil
}
