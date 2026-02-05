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
	pid, err := readPIDFile()
	if err != nil {
		return err
	}
	if err := checkProcessRunning(pid); err != nil {
		return err
	}
	return verifyDevPodProcess(pid)
}

func readPIDFile() (int, error) {
	pidBytes, err := os.ReadFile(pidFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, fmt.Errorf("daemon not running: pid file not found")
		}
		return 0, fmt.Errorf("failed to read pid file: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
	if err != nil {
		return 0, fmt.Errorf("invalid pid file content: %w", err)
	}
	return pid, nil
}

func checkProcessRunning(pid int) error {
	process, _ := os.FindProcess(pid)
	// Signal 0 checks if process exists without sending an actual signal
	if err := process.Signal(syscall.Signal(0)); err != nil {
		return fmt.Errorf("daemon not running (pid %d): %w", pid, err)
	}
	return nil
}

func verifyDevPodProcess(pid int) error {
	cmdline, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return fmt.Errorf("failed to verify daemon process: %w", err)
	}
	// Extract the executable
	parts := strings.Split(string(cmdline), "\x00")
	if len(parts) == 0 || !strings.Contains(parts[0], "devpod") {
		return fmt.Errorf("pid %d is not devpod daemon", pid)
	}
	return nil
}
