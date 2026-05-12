package gitcredentials

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetUser_WorkingDirResolvesIncludeIf(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	projectDir := t.TempDir()
	// #nosec G204 -- projectDir is a temp directory created by the test
	cmd := exec.Command("git", "init", projectDir)
	require.NoError(t, cmd.Run())

	projectConfigPath := filepath.Join(tmpHome, "project.gitconfig")
	require.NoError(t, os.WriteFile(projectConfigPath, []byte(`[user]
	name = Project User
	email = project@example.com
`), 0o600))

	globalConfigPath := filepath.Join(tmpHome, ".gitconfig")
	require.NoError(t, os.WriteFile(globalConfigPath, fmt.Appendf(nil, `[user]
	name = Global User
	email = global@example.com
[includeIf "gitdir:%s/"]
	path = %s
`, projectDir, projectConfigPath), 0o600))

	user, err := GetUser("", projectDir)
	require.NoError(t, err)
	assert.Equal(t, "Project User", user.Name)
	assert.Equal(t, "project@example.com", user.Email)
}

func TestGetUser_EmptyWorkingDirUsesGlobal(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	globalConfigPath := filepath.Join(tmpHome, ".gitconfig")
	require.NoError(t, os.WriteFile(globalConfigPath, []byte(`[user]
	name = Global User
	email = global@example.com
`), 0o600))

	user, err := GetUser("", "")
	require.NoError(t, err)
	assert.Equal(t, "Global User", user.Name)
	assert.Equal(t, "global@example.com", user.Email)
}

func TestGetUser_WorkingDirWithNoMatchingIncludeIfUsesGlobal(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	otherDir := t.TempDir()

	projectConfigPath := filepath.Join(tmpHome, "project.gitconfig")
	require.NoError(t, os.WriteFile(projectConfigPath, []byte(`[user]
	name = Project User
	email = project@example.com
`), 0o600))

	globalConfigPath := filepath.Join(tmpHome, ".gitconfig")
	require.NoError(t, os.WriteFile(globalConfigPath, fmt.Appendf(nil, `[user]
	name = Global User
	email = global@example.com
[includeIf "gitdir:%s/"]
	path = %s
`, otherDir, projectConfigPath), 0o600))

	// workingDir does not match the includeIf pattern — global values should come through
	projectDir := t.TempDir()
	// #nosec G204 -- projectDir is a temp directory created by the test
	cmd := exec.Command("git", "init", projectDir)
	require.NoError(t, cmd.Run())

	user, err := GetUser("", projectDir)
	require.NoError(t, err)
	assert.Equal(t, "Global User", user.Name)
	assert.Equal(t, "global@example.com", user.Email)
}
