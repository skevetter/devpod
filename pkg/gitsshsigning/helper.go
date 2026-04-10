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
		log.Errorf("Failed updating git configuration: %v", err)
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

	// Always remove any existing devpod-managed signing config and rewrite
	// with the current key. The previous guard (checking whether the program
	// line already existed) would silently skip key updates after unclean
	// shutdowns or key rotations.
	newConfig := fmt.Sprintf(GitConfigTemplate, gitSigningKey)
	newContent := removeSignatureHelper(configContent) + newConfig
	if err := writeGitConfig(gitConfigPath, newContent, userName); err != nil {
		return err
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
	type sectionKind int
	const (
		sectionNone sectionKind = iota
		sectionGpgSSH
		sectionGpg
		sectionUser
	)

	current := sectionNone
	var buf []string
	var out []string

	flush := func() {
		switch current {
		case sectionGpgSSH:
			out = append(out, filterSection(buf, func(trimmed string) bool {
				return strings.HasPrefix(trimmed, "program = "+pkgconfig.SSHSignatureHelperName)
			})...)
		case sectionGpg:
			out = append(out, filterSection(buf, func(trimmed string) bool {
				return strings.HasPrefix(trimmed, "format = ssh")
			})...)
		case sectionUser:
			// Only strip [user] sections that contain nothing but signingkey
			// entries — these are the ones appended by GitConfigTemplate.
			// Sections with other entries (name, email, etc.) are user-owned
			// and must be preserved intact to avoid data loss.
			if isDevpodOnlyUserSection(buf) {
				// Drop the entire section — it was appended by devpod.
			} else {
				out = append(out, buf...)
			}
		}
		buf = nil
	}

	for line := range strings.Lines(content) {
		line = strings.TrimRight(line, "\n")
		trimmed := strings.TrimSpace(line)

		if isSectionHeader(trimmed) {
			if current != sectionNone {
				flush()
			}
			switch trimmed {
			case `[gpg "ssh"]`:
				current = sectionGpgSSH
			case "[gpg]":
				current = sectionGpg
			case "[user]":
				current = sectionUser
			default:
				current = sectionNone
			}
			if current != sectionNone {
				buf = append(buf, line)
				continue
			}
		}

		if current != sectionNone {
			buf = append(buf, line)
			continue
		}

		out = append(out, line)
	}

	if current != sectionNone {
		flush()
	}

	return strings.Join(out, "\n")
}

func isSectionHeader(trimmed string) bool {
	return len(trimmed) > 0 && trimmed[0] == '['
}

// isDevpodOnlyUserSection returns true when a buffered [user] section contains
// nothing but signingkey entries. Such sections are appended by GitConfigTemplate
// and are safe to remove. Sections with other entries (name, email, etc.) belong
// to the user and must be preserved.
func isDevpodOnlyUserSection(lines []string) bool {
	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "signingkey = ") {
			return false
		}
	}
	return true
}

// filterSection removes lines matching the predicate from a buffered section.
// Returns the header + remaining lines, or nil if no lines remain after filtering.
func filterSection(lines []string, isManaged func(string) bool) []string {
	if len(lines) == 0 {
		return nil
	}
	var kept []string
	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if !isManaged(trimmed) {
			kept = append(kept, line)
		}
	}
	if len(kept) == 0 {
		return nil
	}
	return append([]string{lines[0]}, kept...)
}
