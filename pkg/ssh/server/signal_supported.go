//go:build !windows

package server

import (
	"os"
	"syscall"

	"github.com/skevetter/log"
	"github.com/skevetter/ssh"
)

func cmdSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

func forwardSignal(log log.Logger, sig ssh.Signal, proc *os.Process) {
	s := osSignalFrom(sig)
	log.Debugf("forwarding signal %s to process %d", s, proc.Pid)
	if err := proc.Signal(s); err != nil {
		log.Debugf("failed to signal process: %v", err)
	}
}

// osSignalFrom maps SSH signal names to OS signals.
// Signal names follow RFC 4254 §6.10.
//   - https://datatracker.ietf.org/doc/html/rfc4254#section-6.10
func osSignalFrom(sig ssh.Signal) os.Signal {
	m := map[ssh.Signal]os.Signal{
		ssh.SIGINT:  syscall.SIGINT,
		ssh.SIGTERM: syscall.SIGTERM,
		ssh.SIGQUIT: syscall.SIGQUIT,
		ssh.SIGHUP:  syscall.SIGHUP,
		ssh.SIGKILL: syscall.SIGKILL,
		ssh.SIGUSR1: syscall.SIGUSR1,
		ssh.SIGUSR2: syscall.SIGUSR2,
	}
	if s, ok := m[sig]; ok {
		return s
	}
	return syscall.SIGTERM
}
