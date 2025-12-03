package network_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/skevetter/devpod/pkg/daemon/workspace/network"
	"github.com/stretchr/testify/suite"
)

type IntegrationTestSuite struct {
	suite.Suite
}

func (s *IntegrationTestSuite) TestTransportFromEnvironment() {
	// Test that we can create transports based on environment
	stdin := bytes.NewReader([]byte{})
	stdout := &bytes.Buffer{}

	// Stdio transport
	stdioTransport := network.NewStdioTransport(stdin, stdout)
	s.NotNil(stdioTransport)

	// HTTP transport
	httpTransport := network.NewHTTPTransport("localhost", "8080")
	s.NotNil(httpTransport)

	// Fallback transport
	fallbackTransport := network.NewFallbackTransport(httpTransport, stdioTransport)
	s.NotNil(fallbackTransport)
}

func (s *IntegrationTestSuite) TestHTTPTunnelClientParsing() {
	// Test parsing of HTTP tunnel client address
	testCases := []struct {
		input    string
		expected []string
	}{
		{"localhost:8080", []string{"localhost", "8080"}},
		{"127.0.0.1:12049", []string{"127.0.0.1", "12049"}},
		{"invalid", nil},
	}

	for _, tc := range testCases {
		parts := strings.Split(tc.input, ":")
		if tc.expected == nil {
			s.NotEqual(2, len(parts), "Should not parse: %s", tc.input)
		} else {
			if len(parts) == 2 {
				s.Equal(tc.expected[0], parts[0])
				s.Equal(tc.expected[1], parts[1])
			}
		}
	}
}

func (s *IntegrationTestSuite) TestEnvironmentVariables() {
	// Test that environment variables can be used
	_ = os.Setenv("TEST_HTTP_CLIENT", "localhost:8080")
	defer func() { _ = os.Unsetenv("TEST_HTTP_CLIENT") }()

	client := os.Getenv("TEST_HTTP_CLIENT")
	s.Equal("localhost:8080", client)

	parts := strings.Split(client, ":")
	s.Equal(2, len(parts))
}

func TestIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(IntegrationTestSuite))
}
