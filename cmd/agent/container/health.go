package container

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

type HealthCmd struct{}

func NewHealthCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "Check if the agent daemon is healthy",
		Args:  cobra.NoArgs,
		RunE:  (&HealthCmd{}).Run,
	}
}

func (cmd *HealthCmd) Run(c *cobra.Command, args []string) error {
	pidBytes, err := os.ReadFile(pidFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("daemon not running: pid file not found")
		}
		return fmt.Errorf("failed to read pid file: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
	if err != nil {
		return fmt.Errorf("invalid pid file content: %w", err)
	}
	process, _ := os.FindProcess(pid)
	// Signal 0 checks if process exists without sending an actual signal
	if err := process.Signal(syscall.Signal(0)); err != nil {
		return fmt.Errorf("daemon not running (pid %d): %w", pid, err)
	}
	// Verify process is the devpod daemon by checking cmdline (Linux-specific)
	// /proc/*/cmdline uses null bytes as argument separators
	cmdline, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return fmt.Errorf("failed to verify daemon process: %w", err)
	}
	// Extract the executable (first argument before null byte)
	parts := strings.Split(string(cmdline), "\x00")
	if len(parts) == 0 || !strings.Contains(parts[0], "devpod") {
		return fmt.Errorf("pid %d is not devpod daemon", pid)
	}
	return nil
}
