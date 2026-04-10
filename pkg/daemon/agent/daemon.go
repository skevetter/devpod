package agent

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
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

// isSystemdAvailable returns true if systemd is the running init system.
// Checking /run/systemd/system is the systemd-recommended approach and
// correctly returns false inside containers or WSL where systemd is not PID 1.
func isSystemdAvailable() bool {
	fi, err := os.Stat("/run/systemd/system")
	if err != nil {
		return false
	}
	return fi.IsDir()
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

// quoteSystemdArg wraps an argument in double quotes if it contains characters
// that require quoting in systemd unit files. Literal percent signs are escaped
// as %% to prevent systemd specifier expansion.
func quoteSystemdArg(arg string) string {
	arg = strings.ReplaceAll(arg, "%", "%%")
	if strings.ContainsAny(arg, " \t\"\\") {
		arg = strings.ReplaceAll(arg, "\\", "\\\\")
		arg = strings.ReplaceAll(arg, "\"", "\\\"")
		return "\"" + arg + "\""
	}
	return arg
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

	if !isSystemdAvailable() {
		log.Warnf("systemd not available, falling back to background process")
		return startFallbackDaemon(executable, args, log)
	}

	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = quoteSystemdArg(a)
	}
	unitContent := systemdUnitContents(strings.Join(quoted, " "))

	needsReload := false
	if !isServiceInstalled() {
		needsReload = true
	} else {
		existing, err := os.ReadFile(serviceFilePath())
		if err != nil || string(existing) != unitContent {
			needsReload = true
		}
	}

	if needsReload {
		//nolint:gosec // systemd unit files must be world-readable (0644)
		if err := os.WriteFile(
			serviceFilePath(), []byte(unitContent), 0o644,
		); err != nil {
			return fmt.Errorf("write service file: %w", err)
		}

		if out, err := exec.Command(
			"systemctl", "daemon-reload",
		).CombinedOutput(); err != nil {
			return fmt.Errorf("systemctl daemon-reload: %s: %w", string(out), err)
		}
	}

	// Always enable so the service starts on boot, even if it was previously disabled.
	//nolint:gosec // BinaryName is a compile-time constant, not tainted input
	if out, err := exec.Command("systemctl", "enable", pkgconfig.BinaryName).
		CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl enable: %s: %w", string(out), err)
	}

	// Restart if the unit file changed, otherwise just ensure it's running.
	if needsReload && isServiceRunning() {
		//nolint:gosec // BinaryName is a compile-time constant
		if out, err := exec.Command(
			"systemctl", "restart", pkgconfig.BinaryName,
		).CombinedOutput(); err != nil {
			log.Warnf("Error restarting service: %s: %v", string(out), err)
			return startFallbackDaemon(executable, args, log)
		}
		log.Infof("restarted DevPod daemon with updated config")
	} else if !isServiceRunning() {
		//nolint:gosec // BinaryName is a compile-time constant, not tainted input
		if out, err := exec.Command(
			"systemctl", "start", pkgconfig.BinaryName,
		).CombinedOutput(); err != nil {
			log.Warnf("Error starting service: %s: %v", string(out), err)
			return startFallbackDaemon(executable, args, log)
		}
		log.Infof("installed DevPod daemon into server")
	}

	return nil
}

func startFallbackDaemon(executable string, args []string, log log.Logger) error {
	daemonArgs := args[1:] // strip executable path
	err := command.StartBackgroundOnce(pkgconfig.DaemonProcessName, func() (*exec.Cmd, error) {
		//nolint:gosec // executable is from os.Executable()
		cmd := exec.Command(executable, daemonArgs...)
		return cmd, nil
	})
	if err != nil {
		return fmt.Errorf("start daemon: %w", err)
	}
	log.Infof("started DevPod daemon into server")
	return nil
}

func RemoveDaemon() error {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return fmt.Errorf("unsupported daemon os")
	}

	// Always attempt to stop the fallback background process, regardless of
	// systemd availability. InstallDaemon may have used the fallback path.
	if err := stopFallbackDaemon(); err != nil {
		return fmt.Errorf("stop fallback daemon: %w", err)
	}

	if !isServiceInstalled() {
		return nil
	}

	// stop and disable the service, propagating real errors
	//nolint:gosec // BinaryName is a compile-time constant
	out, err := exec.Command("systemctl", "stop", pkgconfig.BinaryName).
		CombinedOutput()
	if err != nil {
		// "not loaded" means the unit doesn't exist — treat as no-op
		if !strings.Contains(string(out), "not loaded") {
			return fmt.Errorf("systemctl stop: %s: %w", string(out), err)
		}
	}
	//nolint:gosec // BinaryName is a compile-time constant
	out, err = exec.Command("systemctl", "disable", pkgconfig.BinaryName).
		CombinedOutput()
	if err != nil {
		if !strings.Contains(string(out), "not loaded") {
			return fmt.Errorf("systemctl disable: %s: %w", string(out), err)
		}
	}

	if err := os.Remove(serviceFilePath()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove service file: %w", err)
	}

	if out, err := exec.Command("systemctl", "daemon-reload").CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %s: %w", string(out), err)
	}

	return nil
}

// stopFallbackDaemon kills the PID-file-based background process started by
// command.StartBackgroundOnce and removes its PID file. It verifies process
// identity via /proc/{pid}/exe to avoid killing an unrelated process that
// reused the PID after a reboot.
func stopFallbackDaemon() error {
	pidFile := filepath.Join(os.TempDir(), pkgconfig.DaemonProcessName+".pid")
	pidData, err := os.ReadFile(pidFile) // #nosec G304: not user input
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read pid file: %w", err)
	}

	pid := strings.TrimSpace(string(pidData))
	if _, err := strconv.Atoi(pid); err != nil {
		// Corrupt PID file — clean up and move on
		_ = os.Remove(pidFile)
		return nil
	}

	running, err := command.IsRunning(pid)
	if err != nil || !running {
		// Process gone or check failed — stale PID file
		_ = os.Remove(pidFile)
		return nil
	}

	// Verify this is actually our daemon by checking the executable path.
	// After a reboot the PID may belong to an unrelated process.
	if !isDaemonProcess(pid) {
		_ = os.Remove(pidFile)
		return nil
	}

	if err := command.Kill(pid); err != nil {
		return fmt.Errorf("kill fallback daemon (pid %s): %w", pid, err)
	}

	_ = os.Remove(pidFile)
	return nil
}

// isDaemonProcess checks whether the process with the given PID is a DevPod
// daemon by reading /proc/{pid}/exe and verifying it matches our binary name.
func isDaemonProcess(pid string) bool {
	exePath, err := os.Readlink("/proc/" + pid + "/exe")
	if err != nil {
		// Can't verify — assume it's not ours to be safe
		return false
	}
	baseName := filepath.Base(exePath)
	// Handle " (deleted)" suffix when binary was replaced during upgrade
	baseName = strings.TrimSuffix(baseName, " (deleted)")
	return baseName == pkgconfig.BinaryName
}
