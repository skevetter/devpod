//go:build windows

package server

import (
	"fmt"
	"os"
	"os/exec"
)

func startPTY(cmd *exec.Cmd, w, h int) (*os.File, error) {
	return nil, fmt.Errorf("pty is currently not supported on windows")
}

func setWinSize(f *os.File, w, h int) {
}
