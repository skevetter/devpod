//go:build windows

package command

import "os/exec"

func isRunning(pid string) (bool, error) {
	panic("unsupported")
}

func isExpectedProcess(_, _ string) bool {
	return true
}

func setSetsid(_ *exec.Cmd) {}

func kill(pid string) error {
	panic("unsupported")
}
