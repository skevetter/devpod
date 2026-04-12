package gpg

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildForwardArgs(t *testing.T) {
	args := buildForwardArgs("root", "test-context", "test-workspace")
	assert.Contains(t, args, "ssh")
	assert.Contains(t, args, "--gpg-agent-forwarding=true")
	assert.Contains(t, args, "--agent-forwarding=true")
	assert.Contains(t, args, "--user")
	assert.Contains(t, args, "root")
	assert.Contains(t, args, "--context")
	assert.Contains(t, args, "test-context")
	assert.Contains(t, args, "test-workspace")
	assert.Contains(t, args, "--command")
	assert.Contains(t, args, "sleep infinity")
}
