package gitsshsigning

import (
	"os/exec"
	"strings"
)

const (
	GPGFormatConfigKey       = "gpg.format"
	UsersSigningKeyConfigKey = "user.signingkey"
	GPGFormatSSH             = "ssh"
)

// ExtractGitConfiguration is used for extracting values from users local .gitconfig
// that are needed to setup devpod-ssh-signature helper inside the workspace.
// When workingDir is non-empty the git commands run there so that includeIf "gitdir:..."
// directives are evaluated relative to the project root.
func ExtractGitConfiguration(workingDir string) (string, string, error) {
	format, err := readGitConfigValue(GPGFormatConfigKey, workingDir)
	if err != nil {
		return "", "", err
	}

	signingKey, err := readGitConfigValue(UsersSigningKeyConfigKey, workingDir)
	if err != nil {
		return "", "", err
	}

	return format, signingKey, nil
}

func readGitConfigValue(key string, workingDir string) (string, error) {
	cmd := exec.Command("git", "config", "--get", key)
	if workingDir != "" {
		cmd.Dir = workingDir
	}
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
