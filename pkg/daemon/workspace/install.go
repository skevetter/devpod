package workspace

import (
	"fmt"
	"os"
	"os/exec"
)

// InstallDaemon installs workspace daemon
func InstallDaemon(workspaceDir string) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable: %w", err)
	}

	// Create systemd service or init script
	serviceContent := fmt.Sprintf(`[Unit]
Description=DevPod Workspace Daemon
After=network.target

[Service]
Type=simple
ExecStart=%s agent workspace daemon
Restart=always
WorkingDirectory=%s

[Install]
WantedBy=multi-user.target
`, executable, workspaceDir)

	servicePath := "/etc/systemd/system/devpod-workspace.service"
	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("write service: %w", err)
	}

	// Enable and start service
	if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
		return fmt.Errorf("daemon-reload: %w", err)
	}

	if err := exec.Command("systemctl", "enable", "devpod-workspace").Run(); err != nil {
		return fmt.Errorf("enable service: %w", err)
	}

	return exec.Command("systemctl", "start", "devpod-workspace").Run()
}

// UninstallDaemon removes workspace daemon
func UninstallDaemon() error {
	_ = exec.Command("systemctl", "stop", "devpod-workspace").Run()
	_ = exec.Command("systemctl", "disable", "devpod-workspace").Run()
	_ = os.Remove("/etc/systemd/system/devpod-workspace.service")
	return exec.Command("systemctl", "daemon-reload").Run()
}
