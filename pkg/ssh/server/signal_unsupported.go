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
