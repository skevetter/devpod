package agent

import (
	"os"
	"testing"

	"github.com/stretchr/testify/suite"
)

type AgentTestSuite struct {
	suite.Suite
	originalEnv string
}

func (s *AgentTestSuite) SetupTest() {
	s.originalEnv = os.Getenv(EnvDevPodAgentURL)
}

func (s *AgentTestSuite) TearDownTest() {
	if s.originalEnv != "" {
		os.Setenv(EnvDevPodAgentURL, s.originalEnv)
	} else {
		os.Unsetenv(EnvDevPodAgentURL)
	}
}

func (s *AgentTestSuite) TestDefaultAgentDownloadURL_NoTrailingSlash() {
	os.Setenv(EnvDevPodAgentURL, "https://example.com/releases/latest/download")
	result := DefaultAgentDownloadURL()
	s.Equal("https://example.com/releases/latest/download", result)
}

func (s *AgentTestSuite) TestDefaultAgentDownloadURL_SingleTrailingSlash() {
	os.Setenv(EnvDevPodAgentURL, "https://example.com/releases/latest/download/")
	result := DefaultAgentDownloadURL()
	s.Equal("https://example.com/releases/latest/download", result)
}

func (s *AgentTestSuite) TestDefaultAgentDownloadURL_MultipleTrailingSlashes() {
	os.Setenv(EnvDevPodAgentURL, "https://example.com/releases/latest/download///")
	result := DefaultAgentDownloadURL()
	s.Equal("https://example.com/releases/latest/download", result)
}

func TestAgentSuite(t *testing.T) {
	suite.Run(t, new(AgentTestSuite))
}
