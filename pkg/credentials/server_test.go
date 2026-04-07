package credentials

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/skevetter/devpod/pkg/agent/tunnel"
	"github.com/skevetter/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleGitSSHSignature_GRPCError_ReturnsJSON500(t *testing.T) {
	mock := &mockTunnelClient{
		gitSSHSignatureFunc: func(ctx context.Context, msg *tunnel.Message) (*tunnel.Message, error) {
			return nil, fmt.Errorf("Permission denied")
		},
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/git-ssh-signature",
		strings.NewReader("test payload"),
	)
	w := httptest.NewRecorder()

	err := handleGitSSHSignatureRequest(context.Background(), w, req, mock, log.Discard)
	require.NoError(t, err)

	resp := w.Result()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Contains(t, body["error"], "Permission denied")
}

func TestHandleGitSSHSignature_GRPCSuccess_ReturnsJSON200(t *testing.T) {
	expectedMessage := `{"signature":"abc123"}`
	mock := &mockTunnelClient{
		gitSSHSignatureFunc: func(ctx context.Context, msg *tunnel.Message) (*tunnel.Message, error) {
			return &tunnel.Message{Message: expectedMessage}, nil
		},
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/git-ssh-signature",
		strings.NewReader("test payload"),
	)
	w := httptest.NewRecorder()

	err := handleGitSSHSignatureRequest(context.Background(), w, req, mock, log.Discard)
	require.NoError(t, err)

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "abc123", body["signature"])
}
