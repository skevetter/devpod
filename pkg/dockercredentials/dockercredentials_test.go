package dockercredentials

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/skevetter/log"
	"github.com/stretchr/testify/suite"
)

type ConfigTestSuite struct {
	suite.Suite
}

func TestConfigSuite(t *testing.T) {
	suite.Run(t, new(ConfigTestSuite))
}

func (s *ConfigTestSuite) TestConfigureCredentialsDockerless() {
	tmpDir := s.T().TempDir()
	port := 12345

	dockerConfigDir, err := ConfigureCredentialsDockerless(tmpDir, port, log.Discard)
	s.NoError(err)
	s.NotEmpty(dockerConfigDir)

	// Verify directory was created
	_, err = os.Stat(dockerConfigDir)
	s.NoError(err)

	// Verify credential helper script was created
	helperPath := filepath.Join(dockerConfigDir, "docker-credential-devpod")
	content, err := os.ReadFile(helperPath)
	s.NoError(err)

	// Verify correct shebang for dockerless
	s.Contains(string(content), "#!/.dockerless/bin/sh")
	s.Contains(string(content), "agent docker-credentials")
	s.Contains(string(content), "--port '12345'")

	// Verify environment variables were set
	s.Equal(dockerConfigDir, os.Getenv("DOCKER_CONFIG"))
	s.Contains(os.Getenv("PATH"), dockerConfigDir)

	// Cleanup
	_ = os.RemoveAll(dockerConfigDir)
}

func (s *ConfigTestSuite) TestConfigureCredentialsMachine() {
	tmpDir := s.T().TempDir()
	port := 54321

	dockerConfigDir, err := ConfigureCredentialsMachine(tmpDir, port, log.Discard)
	s.NoError(err)
	s.NotEmpty(dockerConfigDir)

	// Verify directory was created
	_, err = os.Stat(dockerConfigDir)
	s.NoError(err)

	// Verify credential helper script was created
	helperPath := filepath.Join(dockerConfigDir, "docker-credential-devpod")
	content, err := os.ReadFile(helperPath)
	s.NoError(err)

	// Verify correct shebang for machine (standard shell)
	s.Contains(string(content), "#!/bin/sh")
	s.Contains(string(content), "agent docker-credentials")
	s.Contains(string(content), "--port '54321'")

	// Verify environment variables were set
	s.Equal(dockerConfigDir, os.Getenv("DOCKER_CONFIG"))
	s.Contains(os.Getenv("PATH"), dockerConfigDir)

	// Cleanup
	_ = os.RemoveAll(dockerConfigDir)
}

func (s *ConfigTestSuite) TestCredentialsAuthToken() {
	tests := []struct {
		name     string
		creds    Credentials
		expected string
	}{
		{
			name: "username and secret",
			creds: Credentials{
				Username: "user",
				Secret:   "pass",
			},
			expected: "user:pass",
		},
		{
			name: "secret only",
			creds: Credentials{
				Username: "",
				Secret:   "token123",
			},
			expected: "token123",
		},
	}

	for _, tt := range tests {
		result := tt.creds.AuthToken()
		s.Equal(tt.expected, result)
	}
}
