//go:build linux

package agent

import (
	"os"
	"os/signal"
	"syscall"
)

// RunProcessReaper starts a goroutine that reaps zombie child processes.
// This is needed when running as PID 1 (e.g. in a container).
func RunProcessReaper() {
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGCHLD)
		for range c {
			for {
				pid, _ := syscall.Wait4(-1, nil, syscall.WNOHANG, nil)
				if pid <= 0 {
					break
				}
			}
		}
	}()
}
