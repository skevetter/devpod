package tunnelserver

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/skevetter/devpod/pkg/agent/tunnel"
	"github.com/skevetter/devpod/pkg/gitsshsigning"
	"github.com/skevetter/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitSSHSignature_DeserializesPublicKey(t *testing.T) {
	req := gitsshsigning.GitSSHSignatureRequest{
		Content:   "tree abc\nauthor Test\n\ncommit",
		KeyPath:   "/container/tmp/.git_signing_key_tmpXXX",
		PublicKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFake test@test.com",
	}
	reqJSON, err := json.Marshal(req)
	require.NoError(t, err)

	ts := New(log.Discard)

	// This will fail at ssh-keygen (no real key in agent), but we can verify
	// the error is from ssh-keygen execution (not JSON parsing or key resolution),
	// proving the PublicKey was resolved and signing was attempted.
	_, err = ts.GitSSHSignature(context.Background(), &tunnel.Message{
		Message: string(reqJSON),
	})
	require.Error(t, err)
	// The error should be about signing failure (ssh-keygen ran), not about
	// JSON deserialization or key file resolution.
	assert.NotContains(t, err.Error(), "git ssh sign request",
		"request should deserialize without error")
	assert.NotContains(t, err.Error(), "resolve signing key",
		"PublicKey should be resolved to a temp file without error")
	assert.Contains(t, err.Error(), "failed to sign commit",
		"error should be from ssh-keygen execution, proving signing was attempted")
}
