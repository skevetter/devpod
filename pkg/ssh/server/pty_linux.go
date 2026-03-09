//go:build linux

package server

import "syscall"

func ptySysProcAttr(ttyFd uintptr) *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setpgid:   true,
		Setsid:    true,
		Setctty:   true,
		Ctty:      int(ttyFd), //#nosec G115 -- tty fd is always a small non-negative int
		Pdeathsig: syscall.SIGHUP,
	}
}
