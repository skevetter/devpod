package tunnel

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildSSHCommandArgs(t *testing.T) {
	tests := []struct {
		name      string
		context   string
		workspace string
		debug     bool
		extraArgs []string
		expected  []string
	}{
		{
			name:      "basic",
			context:   "default",
			workspace: "my-workspace",
			expected: []string{
				"ssh",
				"--user=root",
				"--agent-forwarding=false",
				"--start-services=false",
				"--context",
				"default",
				"my-workspace",
			},
		},
		{
			name:      "with debug",
			context:   "default",
			workspace: "my-workspace",
			debug:     true,
			expected: []string{
				"ssh",
				"--user=root",
				"--agent-forwarding=false",
				"--start-services=false",
				"--context",
				"default",
				"my-workspace",
				"--debug",
			},
		},
		{
			name:      "with extra args",
			context:   "prod",
			workspace: "ws",
			extraArgs: []string{"--stdio", "--log-output=raw"},
			expected: []string{
				"ssh",
				"--user=root",
				"--agent-forwarding=false",
				"--start-services=false",
				"--context",
				"prod",
				"ws",
				"--stdio",
				"--log-output=raw",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSSHCommandArgs(tt.context, tt.workspace, tt.debug, tt.extraArgs)
			assert.Equal(t, tt.expected, got)
		})
	}
}
