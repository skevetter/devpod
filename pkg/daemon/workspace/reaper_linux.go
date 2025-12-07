//go:build linux

package workspace

import (
	"os"
	"os/signal"
	"syscall"
)

// RunProcessReaper reaps zombie processes when running as PID 1
func RunProcessReaper() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGCHLD)

	for range sigChan {
		for {
			var status syscall.WaitStatus
			pid, err := syscall.Wait4(-1, &status, syscall.WNOHANG, nil)
			if err != nil || pid <= 0 {
				break
			}
		}
	}
}
