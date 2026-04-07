//go:build !windows

package agent

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type SSHAuthSocketSuite struct {
	suite.Suite
	home string
}

func TestSSHAuthSocketSuite(t *testing.T) {
	suite.Run(t, new(SSHAuthSocketSuite))
}

func (s *SSHAuthSocketSuite) SetupSuite() {
	home, err := os.UserHomeDir()
	require.NoError(s.T(), err)
	s.home = home
}

func (s *SSHAuthSocketSuite) TestExpandsTildeFromEnv() {
	s.T().Setenv("SSH_AUTH_SOCK", "~/foo.sock")

	got := GetSSHAuthSocket()
	assert.Equal(s.T(), s.home+"/foo.sock", got)
}

func (s *SSHAuthSocketSuite) TestExpandsBareTilde() {
	s.T().Setenv("SSH_AUTH_SOCK", "~")

	got := GetSSHAuthSocket()
	assert.Equal(s.T(), s.home, got)
}

func (s *SSHAuthSocketSuite) TestAbsolutePathPassthrough() {
	s.T().Setenv("SSH_AUTH_SOCK", "/tmp/ssh-agent.sock")

	got := GetSSHAuthSocket()
	assert.Equal(s.T(), "/tmp/ssh-agent.sock", got)
}

func (s *SSHAuthSocketSuite) TestEmptyReturnsEmpty() {
	s.T().Setenv("SSH_AUTH_SOCK", "")

	got := GetSSHAuthSocket()
	assert.Empty(s.T(), got)
}
