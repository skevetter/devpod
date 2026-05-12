package gitsshsigning

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractGitConfiguration_WorkingDirResolvesIncludeIf(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	projectDir := t.TempDir()
	// #nosec G204 -- projectDir is a temp directory created by the test
	cmd := exec.Command("git", "init", projectDir)
	require.NoError(t, cmd.Run())

	projectConfigPath := filepath.Join(tmpHome, "project.gitconfig")
	require.NoError(t, os.WriteFile(projectConfigPath, []byte(`[gpg]
	format = ssh
[user]
	signingkey = /path/to/signing.pub
`), 0o600))

	globalConfigPath := filepath.Join(tmpHome, ".gitconfig")
	require.NoError(t, os.WriteFile(globalConfigPath, fmt.Appendf(nil, `[includeIf "gitdir:%s/"]
	path = %s
`, projectDir, projectConfigPath), 0o600))

	format, signingKey, err := ExtractGitConfiguration(projectDir)
	require.NoError(t, err)
	assert.Equal(t, GPGFormatSSH, format)
	assert.Equal(t, "/path/to/signing.pub", signingKey)
}

func TestExtractGitConfiguration_EmptyWorkingDir_NoSigningConfig(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	globalConfigPath := filepath.Join(tmpHome, ".gitconfig")
	require.NoError(t, os.WriteFile(globalConfigPath, []byte(`[user]
	email = global@example.com
`), 0o600))

	_, _, err := ExtractGitConfiguration("")
	assert.Error(t, err)
}
