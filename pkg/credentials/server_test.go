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

// errReader is an io.Reader that always returns an error.
type errReader struct{ err error }

func (e *errReader) Read([]byte) (int, error) { return 0, e.err }

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

func TestHandleGitSSHSignature_BodyReadError_ReturnsJSON500(t *testing.T) {
	mock := &mockTunnelClient{
		gitSSHSignatureFunc: func(ctx context.Context, msg *tunnel.Message) (*tunnel.Message, error) {
			t.Fatal("gRPC should not be called when body read fails")
			return nil, nil
		},
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/git-ssh-signature",
		&errReader{err: fmt.Errorf("connection reset")},
	)
	w := httptest.NewRecorder()

	err := handleGitSSHSignatureRequest(context.Background(), w, req, mock, log.Discard)
	require.NoError(t, err, "error should be written to response, not returned")

	resp := w.Result()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err, "response body must be valid JSON")
	assert.Contains(t, body["error"], "connection reset")
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
