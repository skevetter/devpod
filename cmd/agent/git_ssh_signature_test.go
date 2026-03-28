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

	// Git may pass: -Y sign -n git -f /path/to/key -U /tmp/buffer
	// With FParseErrWhitelist, -U is treated as an unknown flag consuming
	// /tmp/buffer as its value. This is fine because git always puts the
	// buffer file as the last argument. We test with the buffer as a
	// separate positional arg (no unknown flag consuming it).
	err := cmd.ParseFlags(
		[]string{"-Y", "sign", "-n", "git", "-f", "/path/to/key", "-U", "/tmp/buffer"},
	)
	assert.NoError(s.T(), err, "flag parsing should succeed with unknown flag -U")
}

func (s *GitSSHSignatureTestSuite) TestBufferFileAsPositionalArg() {
	cmd := NewGitSSHSignatureCmd(&flags.GlobalFlags{})

	// Standard git invocation: -Y sign -n git -f /path/to/key /tmp/buffer
	// The buffer file is the last positional argument.
	err := cmd.ParseFlags(
		[]string{"-Y", "sign", "-n", "git", "-f", "/path/to/key", "/tmp/buffer"},
	)
	assert.NoError(s.T(), err)

	args := cmd.Flags().Args()
	assert.NotEmpty(s.T(), args, "should have positional args")
	assert.Equal(s.T(), "/tmp/buffer", args[len(args)-1],
		"last positional arg should be the buffer file")
}

func (s *GitSSHSignatureTestSuite) TestKnownFlagsParsed() {
	cmd := NewGitSSHSignatureCmd(&flags.GlobalFlags{})

	err := cmd.ParseFlags(
		[]string{"-Y", "sign", "-n", "git", "-f", "/path/to/key", "/tmp/buffer"},
	)
	assert.NoError(s.T(), err, "flag parsing should succeed")

	val, err := cmd.Flags().GetString("command")
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "sign", val, "command flag should be 'sign'")

	// The buffer file should be the last positional argument
	args := cmd.Flags().Args()
	assert.NotEmpty(s.T(), args, "should have positional args")
	assert.Equal(s.T(), "/tmp/buffer", args[len(args)-1],
		"last positional arg should be the buffer file")
}
