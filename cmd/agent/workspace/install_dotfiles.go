package workspace

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/cmd/flags"
	"github.com/skevetter/devpod/pkg/git"
	"github.com/skevetter/log"
	"github.com/spf13/cobra"
)

// InstallDotfilesCmd holds the installDotfiles cmd flags
type InstallDotfilesCmd struct {
	*flags.GlobalFlags

	Repository            string
	InstallScript         string
	StrictHostKeyChecking bool
}

// NewInstallDotfilesCmd creates a new command
func NewInstallDotfilesCmd(flags *flags.GlobalFlags) *cobra.Command {
	cmd := &InstallDotfilesCmd{
		GlobalFlags: flags,
	}
	installDotfilesCmd := &cobra.Command{
		Use:   "install-dotfiles",
		Short: "installs input dotfiles in the container",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return cmd.Run(context.Background())
		},
	}
	installDotfilesCmd.Flags().StringVar(&cmd.Repository, "repository", "", "The dotfiles repository")
	installDotfilesCmd.Flags().StringVar(&cmd.InstallScript, "install-script", "", "The dotfiles install command to execute")
	installDotfilesCmd.Flags().BoolVar(&cmd.StrictHostKeyChecking, "strict-host-key-checking", false, "Set to enable strict host key checking for git cloning via SSH")
	return installDotfilesCmd
}

// Run runs the command logic
func (cmd *InstallDotfilesCmd) Run(ctx context.Context) error {
	logger := log.Default.ErrorStreamOnly()
	targetDir := filepath.Join(os.Getenv("HOME"), "dotfiles")

	_, err := os.Stat(targetDir)
	if err != nil {
		logger.Infof("Cloning dotfiles %s", cmd.Repository)

		gitInfo := git.NormalizeRepositoryGitInfo(cmd.Repository)
		if err := git.CloneRepository(ctx, gitInfo, targetDir, "", cmd.StrictHostKeyChecking, logger); err != nil {
			return err
		}
	} else {
		logger.Info("dotfiles already set up, skipping cloning")
	}

	logger.Debugf("Entering dotfiles directory")

	err = os.Chdir(targetDir)
	if err != nil {
		return err
	}

	if cmd.InstallScript != "" {
		logger.Infof("Executing install script %s", cmd.InstallScript)
		command := "./" + strings.TrimPrefix(cmd.InstallScript, "./")

		err := ensureExecutable(command)
		if err != nil {
			return fmt.Errorf("failed to make install script %s executable %w", command, err)
		}

		scriptCmd := exec.Command(command)
		writer := logger.Writer(logrus.InfoLevel, false)
		scriptCmd.Stdout = writer
		scriptCmd.Stderr = writer

		return scriptCmd.Run()
	}

	logger.Debugf("Install script not specified, trying known locations")

	return setupDotfiles(logger)
}

var installScriptPaths = []string{
	"./install.sh",
	"./install",
	"./bootstrap.sh",
	"./bootstrap",
	"./script/bootstrap",
	"./setup.sh",
	"./setup",
	"./setup/setup",
}

func setupDotfiles(logger log.Logger) error {
	installScriptPaths = slices.DeleteFunc(installScriptPaths, func(path string) bool {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return true
		}

		return false
	})

	for _, installScriptPath := range installScriptPaths {
		writer := logger.Writer(logrus.InfoLevel, false)
		err := ensureExecutable(installScriptPath)
		if err != nil {
			logger.WithFields(logrus.Fields{
				"scriptPath": installScriptPath,
				"error":      err,
			}).Debug("install script not found")
			continue
		}

		logger.WithFields(logrus.Fields{
			"scriptPath": installScriptPath,
		}).Debug("executing dotfile install script")
		scriptCmd := exec.Command(installScriptPath)
		scriptCmd.Stdout = writer
		scriptCmd.Stderr = writer
		if err := scriptCmd.Run(); err != nil {
			logger.WithFields(logrus.Fields{
				"error": err,
			}).Debug("script execution failed")
			continue
		}

		// exit after first successful script
		logger.Debug("install script executed")
		return nil
	}

	logger.Info("Finished script locations, trying to link the files")

	files, err := os.ReadDir(".")
	if err != nil {
		return err
	}

	pwd, err := os.Getwd()
	if err != nil {
		return err
	}

	// link dotfiles in directory to home
	for _, file := range files {
		if strings.HasPrefix(file.Name(), ".") && !file.IsDir() {
			logger.Debugf("linking %s in home", file.Name())

			// remove existing symlink and relink
			if _, err := os.Lstat(filepath.Join(os.Getenv("HOME"), file.Name())); err == nil {
				_ = os.Remove(filepath.Join(os.Getenv("HOME"), file.Name()))
			}
			err = os.Symlink(filepath.Join(pwd, file.Name()), filepath.Join(os.Getenv("HOME"), file.Name()))
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func ensureExecutable(path string) error {
	checkCmd := exec.Command("test", "-f", path)
	err := checkCmd.Run()
	if err != nil {
		return fmt.Errorf("install script %s not found %w", path, err)
	}

	chmodCmd := exec.Command("chmod", "+x", path)
	err = chmodCmd.Run()
	if err != nil {
		return fmt.Errorf("failed to make install script %s executable %w", path, err)
	}

	return nil
}
