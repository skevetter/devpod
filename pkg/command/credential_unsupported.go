//go:build windows

package command

import "os/exec"

func ForUser(cmd *exec.Cmd, userName string) error {
	return nil
}
