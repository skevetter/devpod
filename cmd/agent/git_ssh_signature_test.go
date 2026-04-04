package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type GitSSHSignatureTestSuite struct {
	suite.Suite
}

func TestGitSSHSignatureSuite(t *testing.T) {
	suite.Run(t, new(GitSSHSignatureTestSuite))
}

func (s *GitSSHSignatureTestSuite) TestParseBasicSignArgs() {
	args := []string{"-Y", "sign", "-n", "git", "-f", "/path/to/key.pub", "/tmp/buffer"}
	result := parseSSHKeygenArgs(args)
	assert.Equal(s.T(), "sign", result.command)
	assert.Equal(s.T(), "git", result.namespace)
	assert.Equal(s.T(), "/path/to/key.pub", result.certPath)
	assert.Equal(s.T(), "/tmp/buffer", result.bufferFile)
}

func (s *GitSSHSignatureTestSuite) TestParseWithAgentFlag() {
	// When the signing key is loaded in the ssh-agent, git passes -U (a boolean
	// "use agent" flag) immediately before the buffer file. The buffer file must
	// still be recognised as the last non-flag argument.
	args := []string{"-Y", "sign", "-n", "git", "-f", "/path/to/key.pub", "-U", "/tmp/buffer"}
	result := parseSSHKeygenArgs(args)
	assert.Equal(s.T(), "sign", result.command)
	assert.Equal(s.T(), "/path/to/key.pub", result.certPath)
	assert.Equal(s.T(), "/tmp/buffer", result.bufferFile)
}

func (s *GitSSHSignatureTestSuite) TestParseNonSignCommand() {
	args := []string{"-Y", "verify", "-n", "git", "-f", "/path/to/key.pub", "/tmp/buffer"}
	result := parseSSHKeygenArgs(args)
	assert.Equal(s.T(), "verify", result.command)
}

func (s *GitSSHSignatureTestSuite) TestParseMissingBufferFile() {
	// All args end in a flag — no buffer file present.
	args := []string{"-Y", "sign", "-n", "git", "-f", "/path/to/key.pub", "-U"}
	result := parseSSHKeygenArgs(args)
	assert.Equal(s.T(), "", result.bufferFile)
}

func (s *GitSSHSignatureTestSuite) TestParseDefaultsToSign() {
	// If -Y is absent the command defaults to "sign".
	args := []string{"-n", "git", "-f", "/path/to/key.pub", "/tmp/buffer"}
	result := parseSSHKeygenArgs(args)
	assert.Equal(s.T(), "sign", result.command)
	assert.Equal(s.T(), "/tmp/buffer", result.bufferFile)
}
