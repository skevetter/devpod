package workspace

import (
	"path/filepath"

	"github.com/skevetter/devpod/pkg/daemon/workspace/network"
)

type SshConfig struct {
	Workdir string `json:"workdir,omitempty"`
	User    string `json:"user,omitempty"`
}

type PlatformConfig struct {
	AccessKey     string `json:"accessKey,omitempty"`
	PlatformHost  string `json:"platformHost,omitempty"`
	WorkspaceHost string `json:"workspaceHost,omitempty"`
}

// DaemonConfig holds workspace daemon configuration
type DaemonConfig struct {
	Platform  PlatformConfig          `json:"platform,omitempty"`
	Ssh       SshConfig               `json:"ssh,omitempty"`
	Timeout   string                  `json:"timeout,omitempty"`
	Tailscale network.TailscaleConfig `json:"tailscale,omitempty"`
}

// Validate validates the configuration
func (c *DaemonConfig) Validate() error {
	return nil
}

// DefaultConfig returns default configuration
func DefaultConfig() *DaemonConfig {
	return &DaemonConfig{
		Tailscale: network.TailscaleConfig{
			Enabled:    false,
			StateDir:   filepath.Join(RootDir, "tailscale"),
			ControlURL: "https://controlplane.tailscale.com",
		},
		Timeout: "1h",
	}
}
