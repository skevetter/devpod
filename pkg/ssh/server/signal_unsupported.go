//go:build windows

package server

import (
	"os"
	"syscall"

	"github.com/skevetter/log"
	"github.com/skevetter/ssh"
)

func cmdSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}

func forwardSignal(log log.Logger, sig ssh.Signal, proc *os.Process) {
	log.Debugf("signal forwarding not supported on windows, ignoring %s", sig)
}

// osSignalFrom returns SIGTERM as a safe default on platforms (e.g., Windows)
// where SSH signal mapping is not meaningful. The parameter is intentionally
// unused to maintain API compatibility with the Unix implementation.
func osSignalFrom(_ ssh.Signal) os.Signal {
	return syscall.SIGTERM
}
