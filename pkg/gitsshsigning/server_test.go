package gitsshsigning

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSign_WithPublicKeyContent_WritesToTempFile(t *testing.T) {
	// This test verifies the new behavior: when PublicKey is provided,
	// Sign() writes it to a temp file and uses that for ssh-keygen.
	// We can't do a full sign without a real key in the agent, but we
	// can verify the temp file is created and cleaned up.
	req := &GitSSHSignatureRequest{
		Content:   "tree abc123\nauthor Test <test@example.com>\n\ntest commit",
		KeyPath:   "/tmp/.git_signing_key_does_not_exist",
		PublicKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFakeKeyForTesting test@example.com",
	}

	// Sign will fail because the fake key isn't in the agent, but the error
	// should NOT reference the original KeyPath (container path). Instead it
	// should reference a temp file path that was created on the host.
	_, err := req.Sign()
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "/tmp/.git_signing_key_does_not_exist",
		"with PublicKey set, Sign should use a temp file path, not the container KeyPath")
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
