package local

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewTailscaleClient(t *testing.T) {
	client := NewTailscaleClient("test-client", "test-key", "/tmp/test")
	assert.NotNil(t, client)
	assert.NotNil(t, client.server)
}

func TestTailscaleClientCreation(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
		authKey  string
		stateDir string
	}{
		{
			name:     "valid config",
			hostname: "devpod-client",
			authKey:  "tskey-test",
			stateDir: "/tmp/devpod-tailscale",
		},
		{
			name:     "empty hostname",
			hostname: "",
			authKey:  "key",
			stateDir: "/tmp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewTailscaleClient(tt.hostname, tt.authKey, tt.stateDir)
			assert.NotNil(t, client)
			assert.NotNil(t, client.server)
		})
	}
}
