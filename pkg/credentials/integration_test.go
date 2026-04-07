package credentials

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/skevetter/devpod/pkg/agent/tunnel"
	"github.com/skevetter/devpod/pkg/gitsshsigning"
	"github.com/skevetter/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_SigningFailure_SurfacesServerError(t *testing.T) {
	mock := &mockTunnelClient{
		gitSSHSignatureFunc: func(ctx context.Context, msg *tunnel.Message) (*tunnel.Message, error) {
			return nil, fmt.Errorf(
				"failed to sign commit: exit status 1, stderr: Permission denied (publickey)",
			)
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := handleGitSSHSignatureRequest(context.Background(), w, r, mock, log.Discard)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	gitsshsigning.SetSignatureServerURL(server.URL + "/git-ssh-signature")
	t.Cleanup(func() { gitsshsigning.SetSignatureServerURL("") })

	tmpDir := t.TempDir()
	bufferFile := filepath.Join(tmpDir, "buffer")
	require.NoError(t, os.WriteFile(bufferFile, []byte("commit content"), 0o600))

	err := gitsshsigning.HandleGitSSHProgramCall("/tmp/key.pub", "git", bufferFile, log.Discard)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Permission denied")
	assert.NotContains(t, err.Error(), "invalid character")

	_, statErr := os.Stat(bufferFile + ".sig")
	assert.True(t, os.IsNotExist(statErr), "expected no .sig file to be created")
}

func TestIntegration_SigningSuccess_WritesSigFile(t *testing.T) {
	expectedSig := []byte(
		"-----BEGIN SSH SIGNATURE-----\ntest-signature\n-----END SSH SIGNATURE-----\n",
	)

	mock := &mockTunnelClient{
		gitSSHSignatureFunc: func(ctx context.Context, msg *tunnel.Message) (*tunnel.Message, error) {
			response := gitsshsigning.GitSSHSignatureResponse{Signature: expectedSig}
			jsonBytes, err := json.Marshal(response)
			if err != nil {
				return nil, err
			}
			return &tunnel.Message{Message: string(jsonBytes)}, nil
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := handleGitSSHSignatureRequest(context.Background(), w, r, mock, log.Discard)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	gitsshsigning.SetSignatureServerURL(server.URL + "/git-ssh-signature")
	t.Cleanup(func() { gitsshsigning.SetSignatureServerURL("") })

	tmpDir := t.TempDir()
	bufferFile := filepath.Join(tmpDir, "buffer")
	require.NoError(t, os.WriteFile(bufferFile, []byte("commit content"), 0o600))

	err := gitsshsigning.HandleGitSSHProgramCall("/tmp/key.pub", "git", bufferFile, log.Discard)

	require.NoError(t, err)

	sigContent, readErr := os.ReadFile(
		bufferFile + ".sig",
	) // #nosec G304 -- test file path from t.TempDir
	require.NoError(t, readErr)
	assert.Equal(t, expectedSig, sigContent)
}
