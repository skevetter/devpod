package tunnel

import (
	"encoding/base64"
	"testing"

	"github.com/skevetter/log"
	"github.com/stretchr/testify/assert"
)

const testBaseCommand = "devpod agent container credentials-server --user root"

func TestAddGitSSHSigningKey_ExplicitKey(t *testing.T) {
	command := testBaseCommand
	result := addGitSSHSigningKey(command, "/path/to/key.pub", log.Discard)

	encoded := base64.StdEncoding.EncodeToString([]byte("/path/to/key.pub"))
	assert.Equal(t, command+" --git-user-signing-key "+encoded, result)
}

func TestAddGitSSHSigningKey_ExplicitKeyTakesPrecedence(t *testing.T) {
	// When an explicit key is provided, it should be used regardless
	// of what ExtractGitConfiguration might return from host .gitconfig.
	command := testBaseCommand
	explicitKey := "/explicit/key.pub"
	result := addGitSSHSigningKey(command, explicitKey, log.Discard)

	encoded := base64.StdEncoding.EncodeToString([]byte(explicitKey))
	assert.Equal(t, command+" --git-user-signing-key "+encoded, result)
}

func TestAddGitSSHSigningKey_EmptyExplicitKey_FallsBackToHostConfig(t *testing.T) {
	// Ensure deterministic environment with no host git signing config.
	command := testBaseCommand
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("XDG_CONFIG_HOME", tmpHome)

	result := addGitSSHSigningKey(command, "", log.Discard)

	assert.Equal(t, command, result)
	assert.NotContains(t, result, "--git-user-signing-key")
}

func TestBuildCredentialsCommand_IncludesSigningKey(t *testing.T) {
	opts := RunServicesOptions{
		User:                           "testuser",
		ConfigureGitSSHSignatureHelper: true,
		GitSSHSigningKey:               "/my/key.pub",
		Log:                            log.Discard,
	}
	command := buildCredentialsCommand(opts)

	encoded := base64.StdEncoding.EncodeToString([]byte("/my/key.pub"))
	assert.Contains(t, command, "--git-user-signing-key "+encoded)
	assert.Contains(t, command, "--user testuser")
}

func TestBuildCredentialsCommand_NoSigningKey(t *testing.T) {
	opts := RunServicesOptions{
		User:                           "testuser",
		ConfigureGitSSHSignatureHelper: false,
		Log:                            log.Discard,
	}
	command := buildCredentialsCommand(opts)

	assert.NotContains(t, command, "--git-user-signing-key")
}
