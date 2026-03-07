//go:build !windows

package server

import (
	"math"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"github.com/creack/pty"
)

func startPTY(cmd *exec.Cmd, w, h int) (*os.File, error) {
	return pty.StartWithSize(cmd, &pty.Winsize{Cols: clampUint16(w), Rows: clampUint16(h)})
}

func clampUint16(v int) uint16 {
	if v < 0 {
		return 0
	}
	if v > math.MaxUint16 {
		return math.MaxUint16
	}
	return uint16(v) //#nosec G115
}

func setWinSize(f *os.File, w, h int) {
	ws := struct{ h, w, x, y uint16 }{clampUint16(h), clampUint16(w), 0, 0}
	_, _, _ = syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(syscall.TIOCSWINSZ),
		uintptr(unsafe.Pointer(&ws))) //#nosec G103 -- required for TIOCSWINSZ ioctl
}
