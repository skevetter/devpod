package gitsshsigning

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
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
