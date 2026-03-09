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

// startPTY opens a pty/tty pair, sets SSH_TTY on the command, wires
// stdin/stdout/stderr to the tty, starts the command, and returns the
// pty master file descriptor. The caller is responsible for closing it.
//
// Similar PTY solutions used in other projects:
//   - coder/coder:           https://github.com/coder/coder/blob/main/agent/agentssh/agentssh.go
//   - coder/wsep:            https://github.com/coder/wsep/blob/master/localexec_unix.go#L64
//   - wavetermdev/waveterm:  https://github.com/wavetermdev/waveterm/blob/main/pkg/shellexec/shellexec.go#L168
//   - jumpserver/koko:       https://github.com/jumpserver/koko/blob/dev/pkg/localcommand/local_command.go#L49
//   - daytonaio/daytona:
//     https://github.com/daytonaio/daytona/blob/main/apps/daemon/pkg/toolbox/process/pty/session.go#L61
func startPTY(cmd *exec.Cmd, w, h int) (*os.File, error) {
	ptmx, tty, err := pty.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tty.Close() }()

	if w > 0 || h > 0 {
		if err := pty.Setsize(
			ptmx,
			&pty.Winsize{Cols: clampUint16(w), Rows: clampUint16(h)},
		); err != nil {
			_ = ptmx.Close()
			return nil, err
		}
	}

	cmd.Env = append(cmd.Env, "SSH_TTY="+tty.Name())
	cmd.Stdin = tty
	cmd.Stdout = tty
	cmd.Stderr = tty
	cmd.SysProcAttr = ptySysProcAttr(tty.Fd())

	if err := cmd.Start(); err != nil {
		_ = ptmx.Close()
		return nil, err
	}

	return ptmx, nil
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

func setWinSize(f *os.File, w, h int) error {
	ws := struct{ h, w, x, y uint16 }{clampUint16(h), clampUint16(w), 0, 0}
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(syscall.TIOCSWINSZ),
		uintptr(unsafe.Pointer(&ws))) //#nosec G103 -- required for TIOCSWINSZ ioctl
	if errno != 0 {
		return errno
	}
	return nil
}
