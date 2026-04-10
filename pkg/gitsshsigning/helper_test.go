package gitsshsigning

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type HelperTestSuite struct {
	suite.Suite
}

func TestHelperSuite(t *testing.T) {
	suite.Run(t, new(HelperTestSuite))
}

func (s *HelperTestSuite) TestRemoveSignatureHelper_PreservesUnrelatedGpgConfig() {
	input := strings.Join([]string{
		"[user]", "\tname = Test User", "\temail = test@example.com",
		`[gpg "ssh"]`, "\tprogram = devpod-ssh-signature",
		"[gpg]", "\tformat = ssh", "\tprogram = /usr/bin/gpg2",
		"[commit]", "\tgpgsign = true",
		"[user]", "\tsigningkey = /path/to/key",
	}, "\n")

	result := removeSignatureHelper(input)

	assert.NotContains(s.T(), result, "devpod-ssh-signature")
	assert.Contains(s.T(), result, "[user]")
	assert.Contains(s.T(), result, "[commit]")
	assert.Contains(s.T(), result, "program = /usr/bin/gpg2")
	assert.NotContains(s.T(), result, "format = ssh")
}

func (s *HelperTestSuite) TestRemoveSignatureHelper_RemovesDevpodSections() {
	input := strings.Join([]string{
		"[user]", "\tname = Test User",
		`[gpg "ssh"]`, "\tprogram = devpod-ssh-signature",
		"[gpg]", "\tformat = ssh",
		"[user]", "\tsigningkey = /path/to/key",
	}, "\n")

	result := removeSignatureHelper(input)

	assert.NotContains(s.T(), result, "devpod-ssh-signature")
	assert.NotContains(s.T(), result, "format = ssh")
	assert.Contains(s.T(), result, "Test User")
}

func (s *HelperTestSuite) TestRemoveSignatureHelper_NoGpgSections() {
	input := "[user]\n\tname = Test User\n\temail = test@example.com"

	result := removeSignatureHelper(input)

	assert.Equal(s.T(), input, result)
}

func (s *HelperTestSuite) TestRemoveSignatureHelper_PreservesUserOwnedGpgSSHKeys() {
	input := strings.Join([]string{
		"[user]", "\tname = Test User",
		`[gpg "ssh"]`, "\tprogram = devpod-ssh-signature",
		"\tallowedSignersFile = ~/.ssh/allowed_signers",
		"[commit]", "\tgpgsign = true",
	}, "\n")

	result := removeSignatureHelper(input)

	assert.NotContains(s.T(), result, "devpod-ssh-signature")
	assert.Contains(s.T(), result, `[gpg "ssh"]`,
		"section header should be preserved when user keys remain")
	assert.Contains(s.T(), result, "allowedSignersFile",
		"user-owned key should be preserved")
	assert.Contains(s.T(), result, "[commit]")
}

func (s *HelperTestSuite) TestRemoveSignatureHelper_PreservesUserOwnedSigningKey() {
	// A [user] section with name + signingkey is user-owned; only the
	// devpod-appended [user] section (signingkey-only) should be dropped.
	input := strings.Join([]string{
		"[user]", "\tname = Test User", "\tsigningkey = /my/gpg-key",
		`[gpg "ssh"]`, "\tprogram = devpod-ssh-signature",
		"[gpg]", "\tformat = ssh",
		"[user]", "\tsigningkey = /devpod/injected-key",
	}, "\n")

	result := removeSignatureHelper(input)

	assert.Contains(s.T(), result, "signingkey = /my/gpg-key",
		"user-owned signingkey must be preserved")
	assert.NotContains(s.T(), result, "/devpod/injected-key",
		"devpod-only [user] section should be removed")
	assert.Contains(s.T(), result, "Test User")
}

func TestUpdateGitConfig_Idempotent(t *testing.T) {
	dir := t.TempDir()
	gitConfigPath := filepath.Join(dir, ".gitconfig")

	// First call: writes signing config
	err := updateGitConfig(gitConfigPath, "", "/path/to/key.pub")
	require.NoError(t, err)

	content1, err := os.ReadFile(gitConfigPath) // #nosec G304 -- test path from t.TempDir
	require.NoError(t, err)
	assert.Contains(t, string(content1), "program = devpod-ssh-signature")
	assert.Contains(t, string(content1), "signingkey = /path/to/key.pub")

	// Second call with same config: should be a no-op
	err = updateGitConfig(gitConfigPath, "", "/path/to/key.pub")
	require.NoError(t, err)

	content2, err := os.ReadFile(gitConfigPath) // #nosec G304 -- test path from t.TempDir
	require.NoError(t, err)
	assert.Equal(t, string(content1), string(content2), "second call should not duplicate config")
}

func TestUpdateGitConfig_DifferentKey(t *testing.T) {
	dir := t.TempDir()
	gitConfigPath := filepath.Join(dir, ".gitconfig")

	// First call with key A
	err := updateGitConfig(gitConfigPath, "", "/path/to/keyA.pub")
	require.NoError(t, err)

	// Second call with key B: should update to the new key
	err = updateGitConfig(gitConfigPath, "", "/path/to/keyB.pub")
	require.NoError(t, err)

	content, err := os.ReadFile(gitConfigPath) // #nosec G304 -- test path from t.TempDir
	require.NoError(t, err)
	assert.Contains(t, string(content), "program = devpod-ssh-signature")
	assert.Contains(t, string(content), "signingkey = /path/to/keyB.pub",
		"key should be updated to the new value")
	assert.NotContains(t, string(content), "keyA",
		"old key should be replaced")
}

func (s *HelperTestSuite) TestRemoveSignatureHelper_DropsEmptyGpgSSHSection() {
	input := strings.Join([]string{
		"[user]", "\tname = Test User",
		`[gpg "ssh"]`, "\tprogram = devpod-ssh-signature",
		"[commit]", "\tgpgsign = true",
	}, "\n")

	result := removeSignatureHelper(input)

	assert.NotContains(s.T(), result, "devpod-ssh-signature")
	assert.NotContains(s.T(), result, `[gpg "ssh"]`,
		"empty section should be dropped entirely")
	assert.Contains(s.T(), result, "[commit]")
}
