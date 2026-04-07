//go:build linux

package agent

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// RunProcessReaper reaps zombie child processes until ctx is cancelled.
// This is needed when running as PID 1 (e.g. in a container).
func RunProcessReaper(ctx context.Context) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGCHLD)
	go func() {
		<-ctx.Done()
		signal.Stop(c)
		close(c)
	}()
	for range c {
		for {
			pid, err := syscall.Wait4(-1, nil, syscall.WNOHANG, nil)
			if err == syscall.EINTR {
				continue
			}
			if pid <= 0 {
				break
			}
		}
	}
}
