//go:build !windows

package pty

import (
	"fmt"
	"log"

	"github.com/skevetter/ssh"
	"github.com/u-root/u-root/pkg/termios"
	gossh "golang.org/x/crypto/ssh"
)

// terminalModeFlagNames maps the SSH terminal mode flags to mnemonic
// names used by the termios package.
var terminalModeFlagNames = map[uint8]string{
	gossh.VINTR:         "intr",
	gossh.VQUIT:         "quit",
	gossh.VERASE:        "erase",
	gossh.VKILL:         "kill",
	gossh.VEOF:          "eof",
	gossh.VEOL:          "eol",
	gossh.VEOL2:         "eol2",
	gossh.VSTART:        "start",
	gossh.VSTOP:         "stop",
	gossh.VSUSP:         "susp",
	gossh.VDSUSP:        "dsusp",
	gossh.VREPRINT:      "rprnt",
	gossh.VWERASE:       "werase",
	gossh.VLNEXT:        "lnext",
	gossh.VFLUSH:        "flush",
	gossh.VSWTCH:        "swtch",
	gossh.VSTATUS:       "status",
	gossh.VDISCARD:      "discard",
	gossh.IGNPAR:        "ignpar",
	gossh.PARMRK:        "parmrk",
	gossh.INPCK:         "inpck",
	gossh.ISTRIP:        "istrip",
	gossh.INLCR:         "inlcr",
	gossh.IGNCR:         "igncr",
	gossh.ICRNL:         "icrnl",
	gossh.IUCLC:         "iuclc",
	gossh.IXON:          "ixon",
	gossh.IXANY:         "ixany",
	gossh.IXOFF:         "ixoff",
	gossh.IMAXBEL:       "imaxbel",
	gossh.IUTF8:         "iutf8",
	gossh.ISIG:          "isig",
	gossh.ICANON:        "icanon",
	gossh.XCASE:         "xcase",
	gossh.ECHO:          "echo",
	gossh.ECHOE:         "echoe",
	gossh.ECHOK:         "echok",
	gossh.ECHONL:        "echonl",
	gossh.NOFLSH:        "noflsh",
	gossh.TOSTOP:        "tostop",
	gossh.IEXTEN:        "iexten",
	gossh.ECHOCTL:       "echoctl",
	gossh.ECHOKE:        "echoke",
	gossh.PENDIN:        "pendin",
	gossh.OPOST:         "opost",
	gossh.OLCUC:         "olcuc",
	gossh.ONLCR:         "onlcr",
	gossh.OCRNL:         "ocrnl",
	gossh.ONOCR:         "onocr",
	gossh.ONLRET:        "onlret",
	gossh.CS7:           "cs7",
	gossh.CS8:           "cs8",
	gossh.PARENB:        "parenb",
	gossh.PARODD:        "parodd",
	gossh.TTY_OP_ISPEED: "tty_op_ispeed",
	gossh.TTY_OP_OSPEED: "tty_op_ospeed",
}

// applyTerminalModesToFd applies the terminal settings from the SSH
// request to the given fd.
//
// This is based on code from Tailscale's tailssh package:
// https://github.com/tailscale/tailscale/blob/main/ssh/tailssh/incubator.go
func applyTerminalModesToFd(logger *log.Logger, fd uintptr, req ssh.Pty) error {
	tios, err := termios.GTTY(int(fd)) //#nosec G115 -- fd is a valid file descriptor
	if err != nil {
		return fmt.Errorf("GTTY: %w", err)
	}

	tios.Row = req.Window.Height
	tios.Col = req.Window.Width

	for c, v := range req.Modes {
		applyTerminalMode(logger, tios, c, v)
	}

	if _, err := tios.STTY(int(fd)); err != nil { //#nosec G115 -- fd is a valid file descriptor
		return fmt.Errorf("STTY: %w", err)
	}

	return nil
}

func applyTerminalMode(logger *log.Logger, tios *termios.TTY, c uint8, v uint32) {
	if c == gossh.TTY_OP_ISPEED {
		tios.Ispeed = int(v)
		return
	}
	if c == gossh.TTY_OP_OSPEED {
		tios.Ospeed = int(v)
		return
	}
	k, ok := terminalModeFlagNames[c]
	if !ok {
		if logger != nil {
			logger.Printf("unknown terminal mode: %d", c)
		}
		return
	}
	if _, ok := tios.CC[k]; ok {
		tios.CC[k] = clampUint8(logger, k, v)
		return
	}
	if _, ok := tios.Opts[k]; ok {
		tios.Opts[k] = v > 0
		return
	}
	if logger != nil {
		logger.Printf("unsupported terminal mode: k=%s, c=%d, v=%d", k, c, v)
	}
}

func clampUint8(logger *log.Logger, k string, v uint32) uint8 {
	if v > 255 {
		if logger != nil {
			logger.Printf("terminal mode CC[%s] value %d exceeds uint8 range, clamping", k, v)
		}
		return 255
	}
	return uint8(v) //#nosec G115 -- bounds checked above
}
