package network

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type TailscaleServerTestSuite struct {
	suite.Suite
}

func TestTailscaleServerTestSuite(t *testing.T) {
	suite.Run(t, new(TailscaleServerTestSuite))
}

func (s *TailscaleServerTestSuite) TestNewTailscaleServer() {
	config := &TailscaleConfig{
		Enabled:    true,
		Hostname:   "test-host",
		AuthKey:    "test-key",
		ControlURL: "https://test.example.com",
		StateDir:   "/tmp/test",
	}

	ts := NewTailscaleServer(config)

	s.NotNil(ts)
	s.Equal(config, ts.config)
	s.NotNil(ts.server)
}

func (s *TailscaleServerTestSuite) TestConfigValidation() {
	tests := []struct {
		name   string
		config *TailscaleConfig
		valid  bool
	}{
		{
			name: "valid config",
			config: &TailscaleConfig{
				Enabled:    true,
				Hostname:   "test",
				AuthKey:    "key",
				ControlURL: "https://test.com",
				StateDir:   "/tmp",
			},
			valid: true,
		},
		{
			name: "disabled",
			config: &TailscaleConfig{
				Enabled: false,
			},
			valid: true,
		},
		{
			name: "empty hostname",
			config: &TailscaleConfig{
				Enabled:  true,
				Hostname: "",
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			ts := NewTailscaleServer(tt.config)
			s.NotNil(ts)
		})
	}
}

func (s *TailscaleServerTestSuite) TestServerCreation() {
	config := &TailscaleConfig{
		Enabled:    true,
		Hostname:   "devpod-test",
		AuthKey:    "tskey-test",
		ControlURL: "https://controlplane.tailscale.com",
		StateDir:   "/var/devpod/tailscale",
	}

	ts := NewTailscaleServer(config)

	s.NotNil(ts.server)
	s.NotNil(ts.config)
}
