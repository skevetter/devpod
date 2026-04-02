package agent

import (
	"os"
	"testing"

	"github.com/skevetter/devpod/pkg/config"
	"github.com/stretchr/testify/suite"
)

type AgentTestSuite struct {
	suite.Suite
	originalEnv string
}

func (s *AgentTestSuite) SetupTest() {
	s.originalEnv = os.Getenv(config.EnvAgentURL)
}

func (s *AgentTestSuite) TearDownTest() {
	if s.originalEnv != "" {
		_ = os.Setenv(config.EnvAgentURL, s.originalEnv)
	} else {
		_ = os.Unsetenv(config.EnvAgentURL)
	}
}

func (s *AgentTestSuite) TestDefaultAgentDownloadURL_NoTrailingSlash() {
	_ = os.Setenv(config.EnvAgentURL, "https://example.com/releases/latest/download")
	result := DefaultAgentDownloadURL()
	s.Equal("https://example.com/releases/latest/download", result)
}

func (s *AgentTestSuite) TestDefaultAgentDownloadURL_SingleTrailingSlash() {
	_ = os.Setenv(config.EnvAgentURL, "https://example.com/releases/latest/download/")
	result := DefaultAgentDownloadURL()
	s.Equal("https://example.com/releases/latest/download", result)
}

func (s *AgentTestSuite) TestDefaultAgentDownloadURL_MultipleTrailingSlashes() {
	_ = os.Setenv(config.EnvAgentURL, "https://example.com/releases/latest/download///")
	result := DefaultAgentDownloadURL()
	s.Equal("https://example.com/releases/latest/download", result)
}

func TestAgentSuite(t *testing.T) {
	suite.Run(t, new(AgentTestSuite))
}
