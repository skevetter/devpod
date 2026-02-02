package git

import (
	"fmt"
	"os/exec"

	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/pkg/command"
	"github.com/skevetter/log"
)

func InstallBinary(log log.Logger) error {
	writer := log.Writer(logrus.InfoLevel, false)
	errwriter := log.Writer(logrus.ErrorLevel, false)
	defer func() { _ = writer.Close() }()
	defer func() { _ = errwriter.Close() }()

	// try to install git via apt / apk
	if !command.Exists("apt") && !command.Exists("apk") {
		// TODO: use golang git implementation
		return fmt.Errorf("couldn't find a package manager to install git")
	}

	if command.Exists("apt") {
		log.Infof("Git command is missing, try to install git with apt...")
		cmd := exec.Command("apt", "update")
		cmd.Stdout = writer
		cmd.Stderr = errwriter
		err := cmd.Run()
		if err != nil {
			return fmt.Errorf("run apt update: %w", err)
		}
		cmd = exec.Command("apt", "-y", "install", "git")
		cmd.Stdout = writer
		cmd.Stderr = errwriter
		err = cmd.Run()
		if err != nil {
			return fmt.Errorf("run apt install git -y: %w", err)
		}
	} else if command.Exists("apk") {
		log.Infof("Git command is missing, try to install git with apk...")
		cmd := exec.Command("apk", "update")
		cmd.Stdout = writer
		cmd.Stderr = errwriter
		err := cmd.Run()
		if err != nil {
			return fmt.Errorf("run apk update: %w", err)
		}
		cmd = exec.Command("apk", "add", "git")
		cmd.Stdout = writer
		cmd.Stderr = errwriter
		err = cmd.Run()
		if err != nil {
			return fmt.Errorf("run apk add git: %w", err)
		}
	}

	// is git available now?
	if !command.Exists("git") {
		return fmt.Errorf("couldn't install git")
	}

	log.Donef("installed git")

	return nil
}
