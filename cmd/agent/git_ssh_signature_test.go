package agent

import (
	"testing"

	"github.com/skevetter/devpod/cmd/flags"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type GitSSHSignatureTestSuite struct {
	suite.Suite
}

func TestGitSSHSignatureSuite(t *testing.T) {
	suite.Run(t, new(GitSSHSignatureTestSuite))
}

func (s *GitSSHSignatureTestSuite) TestAcceptsUnknownFlags() {
	cmd := NewGitSSHSignatureCmd(&flags.GlobalFlags{})

	// Git passes: -Y sign -n git -f /path/to/key -U /dev/stdin /tmp/buffer
	// -U is an unknown flag that consumes /dev/stdin as its value.
	// /tmp/buffer remains as a positional argument.
	err := cmd.ParseFlags(
		[]string{
			"-Y",
			"sign",
			"-n",
			"git",
			"-f",
			"/path/to/key",
			"-U",
			"/dev/stdin",
			"/tmp/buffer",
		},
	)
	assert.NoError(s.T(), err, "flag parsing should succeed with unknown flag -U")

	args := cmd.Flags().Args()
	s.Require().NotEmpty(args, "should have positional args")
	assert.Equal(s.T(), "/tmp/buffer", args[len(args)-1],
		"buffer file should be preserved as last positional arg")
}

func (s *GitSSHSignatureTestSuite) TestBufferFileAsPositionalArg() {
	cmd := NewGitSSHSignatureCmd(&flags.GlobalFlags{})

	err := cmd.ParseFlags(
		[]string{"-Y", "sign", "-n", "git", "-f", "/path/to/key", "/tmp/buffer"},
	)
	assert.NoError(s.T(), err)

	args := cmd.Flags().Args()
	s.Require().NotEmpty(args, "should have positional args")
	assert.Equal(s.T(), "/tmp/buffer", args[len(args)-1],
		"last positional arg should be the buffer file")
}

func (s *GitSSHSignatureTestSuite) TestKnownFlagsParsed() {
	cmd := NewGitSSHSignatureCmd(&flags.GlobalFlags{})

	err := cmd.ParseFlags(
		[]string{"-Y", "sign", "-n", "git", "-f", "/path/to/key", "/tmp/buffer"},
	)
	assert.NoError(s.T(), err)

	val, err := cmd.Flags().GetString("command")
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "sign", val, "command flag should be 'sign'")

	args := cmd.Flags().Args()
	s.Require().NotEmpty(args, "should have positional args")
	assert.Equal(s.T(), "/tmp/buffer", args[len(args)-1],
		"last positional arg should be the buffer file")
}
