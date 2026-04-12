package gitsshsigning

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSign_WithPublicKeyContent_WritesToTempFile(t *testing.T) {
	req := &GitSSHSignatureRequest{
		Content:   "tree abc123\nauthor Test <test@example.com>\n\ntest commit",
		KeyPath:   "/tmp/.git_signing_key_does_not_exist",
		PublicKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFakeKeyForTesting test@example.com",
	}

	_, err := req.Sign()
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "/tmp/.git_signing_key_does_not_exist")
}

func TestSign_NonExistentKeyPath_ReturnsError(t *testing.T) {
	req := &GitSSHSignatureRequest{
		Content: "tree abc123\nauthor Test <test@example.com>\n\ntest commit",
		KeyPath: "/tmp/.git_signing_key_does_not_exist",
	}

	_, err := req.Sign()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to sign commit")
}
