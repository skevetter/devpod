package workspace

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
)

// RunSshServer starts SSH server
func RunSshServer(ctx context.Context, d *Daemon, errChan chan<- error, wg *sync.WaitGroup) {
	defer wg.Done()

	if d.config.Ssh.Workdir == "" {
		errChan <- fmt.Errorf("ssh workdir not configured")
		return
	}

	if err := os.Chdir(d.config.Ssh.Workdir); err != nil {
		errChan <- fmt.Errorf("chdir: %w", err)
		return
	}

	cmd := exec.CommandContext(ctx, "/usr/sbin/sshd", "-D", "-e")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	d.log.Info("Starting SSH server")
	if err := cmd.Run(); err != nil && ctx.Err() == nil {
		errChan <- fmt.Errorf("sshd: %w", err)
	}
}
