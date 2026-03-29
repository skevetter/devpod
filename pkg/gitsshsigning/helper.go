package gitsshsigning

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/skevetter/devpod/pkg/command"
	pkgconfig "github.com/skevetter/devpod/pkg/config"
	"github.com/skevetter/devpod/pkg/file"
	"github.com/skevetter/log"
)

const (
	HelperScript = `#!/bin/bash

devpod agent git-ssh-signature "$@"
`
)

var GitConfigTemplate = `
[gpg "ssh"]
	program = ` + pkgconfig.SSHSignatureHelperName + `
[gpg]
	format = ssh
[user]
	signingkey = %s
`

// ConfigureHelper sets up the Git SSH signing helper script and updates the Git configuration for the specified user.
//
// This function:
// - sets user.signingkey git config
// - creates a wrapper script for calling git-ssh-signature
// - users this script as gpg.ssh.program
// This is needed since git expects `gpg.ssh.program` to be an executable.
func ConfigureHelper(userName, gitSigningKey string, log log.Logger) error {
	log.Debug("Creating helper script")
	if err := createHelperScript(); err != nil {
		return err
	}
	log.Debugf("Helper script created. Making it executable.")
	if err := makeScriptExecutable(); err != nil {
		return err
	}
	log.Debugf("Script executable. Getting config path.")
	gitConfigPath, err := getGitConfigPath(userName)
	if err != nil {
		return err
	}
	log.Debugf("Got config path: %v", gitConfigPath)
	if err := updateGitConfig(gitConfigPath, userName, gitSigningKey); err != nil {
		log.Errorf("Failed updating git configuration: %w", err)
		return err
	}

	return nil
}

// RemoveHelper removes the git SSH signing helper script and any related configuration.
func RemoveHelper(userName string) error {
	if err := os.Remove(pkgconfig.SSHSignatureHelperPath); err != nil && !os.IsNotExist(err) {
		return err
	}

	gitConfigPath, err := getGitConfigPath(userName)
	if err != nil {
		return err
	}

	if err := removeGitConfigHelper(gitConfigPath, userName); err != nil {
		return err
	}

	return nil
}

func createHelperScript() error {
	// we do it this way instead of os.Create because we need sudo
	cmd := exec.Command(
		"sudo",
		"bash",
		"-c",
		fmt.Sprintf("echo '%s' > %s", HelperScript, pkgconfig.SSHSignatureHelperPath),
	)
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

func makeScriptExecutable() error {
	cmd := exec.Command("sudo", "chmod", "+x", pkgconfig.SSHSignatureHelperPath) // #nosec G204
	return cmd.Run()
}

func getGitConfigPath(userName string) (string, error) {
	homeDir, err := command.GetHome(userName)
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".gitconfig"), nil
}

func updateGitConfig(gitConfigPath, userName, gitSigningKey string) error {
	configContent, err := readGitConfig(gitConfigPath)
	if err != nil {
		return err
	}

	if !strings.Contains(configContent, "program = "+pkgconfig.SSHSignatureHelperName) {
		newConfig := fmt.Sprintf(GitConfigTemplate, gitSigningKey)
		newContent := removeSignatureHelper(configContent) + newConfig
		if err := writeGitConfig(gitConfigPath, newContent, userName); err != nil {
			return err
		}
	}

	return nil
}

func readGitConfig(gitConfigPath string) (string, error) {
	out, err := os.ReadFile(gitConfigPath)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	return string(out), nil
}

func writeGitConfig(gitConfigPath, content, userName string) error {
	if err := os.WriteFile(gitConfigPath, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write git config: %w", err)
	}
	return file.Chown(userName, gitConfigPath)
}

func removeGitConfigHelper(gitConfigPath, userName string) error {
	configContent, err := readGitConfig(gitConfigPath)
	if err != nil {
		return err
	}

	newContent := removeSignatureHelper(configContent)
	if err := writeGitConfig(gitConfigPath, newContent, userName); err != nil {
		return err
	}

	return nil
}

func removeSignatureHelper(content string) string {
	inGpgSSHSection := false
	inGpgSection := false
	var gpgSSHBuffer []string
	var out []string

	for line := range strings.Lines(content) {
		line = strings.TrimRight(line, "\n")
		trimmed := strings.TrimSpace(line)

		if isSectionHeader(trimmed) {
			if inGpgSSHSection {
				out = append(out, filterGpgSSHSection(gpgSSHBuffer)...)
				gpgSSHBuffer = nil
			}
			inGpgSSHSection = trimmed == `[gpg "ssh"]`
			inGpgSection = trimmed == "[gpg]"
			if inGpgSSHSection {
				gpgSSHBuffer = append(gpgSSHBuffer, line)
				continue
			}
		}

		if inGpgSSHSection {
			gpgSSHBuffer = append(gpgSSHBuffer, line)
			continue
		}

		if !isDevpodManagedGpgKey(inGpgSection, trimmed) {
			out = append(out, line)
		}
	}

	if inGpgSSHSection {
		out = append(out, filterGpgSSHSection(gpgSSHBuffer)...)
	}

	return strings.Join(out, "\n")
}

func isSectionHeader(trimmed string) bool {
	return len(trimmed) > 0 && trimmed[0] == '['
}

func isDevpodManagedGpgKey(inGpgSection bool, trimmed string) bool {
	if !inGpgSection || len(trimmed) == 0 || trimmed[0] == '[' {
		return false
	}
	return strings.HasPrefix(trimmed, "format = ssh")
}

// filterGpgSSHSection removes devpod-managed keys from a buffered [gpg "ssh"]
// section. Returns the header + remaining user keys, or nil if no user keys remain.
func filterGpgSSHSection(lines []string) []string {
	if len(lines) == 0 {
		return nil
	}
	var kept []string
	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "program = "+pkgconfig.SSHSignatureHelperName) {
			kept = append(kept, line)
		}
	}
	if len(kept) == 0 {
		return nil
	}
	return append([]string{lines[0]}, kept...)
}
