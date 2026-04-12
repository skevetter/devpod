package gpg

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildForwardArgs(t *testing.T) {
	got := buildForwardArgs("root", "test-context", "test-workspace")
	expected := []string{
		"ssh",
		"--gpg-agent-forwarding=true",
		"--agent-forwarding=true",
		"--start-services=true",
		"--user", "root",
		"--context", "test-context",
		"test-workspace",
		"--log-output=raw",
		"--command", "sleep infinity",
	}
	assert.Equal(t, expected, got)
}
