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

	_, err = ts.GitSSHSignature(context.Background(), &tunnel.Message{
		Message: string(reqJSON),
	})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "git ssh sign request")
	assert.NotContains(t, err.Error(), "resolve signing key")
	assert.Contains(t, err.Error(), "failed to sign commit")
}
