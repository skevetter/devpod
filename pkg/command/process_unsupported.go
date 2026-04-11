//go:build windows

package command

func isRunning(pid string) (bool, error) {
	panic("unsupported")
}

func isExpectedProcess(_, _ string) bool {
	return true
}

func kill(pid string) error {
	panic("unsupported")
}
