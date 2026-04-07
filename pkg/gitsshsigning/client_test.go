package gitsshsigning

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/skevetter/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSignatureResponse_NonJSONBody(t *testing.T) {
	body := []byte("get git ssh signature: permission denied")
	_, err := parseSignatureResponse(body, log.Discard)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get git ssh signature: permission denied")
}

func TestParseSignatureResponse_ValidJSON(t *testing.T) {
	sig := []byte("ssh-sig-data")
	response := &GitSSHSignatureResponse{Signature: sig}
	body, err := json.Marshal(response)
	require.NoError(t, err)

	result, err := parseSignatureResponse(body, log.Discard)
	require.NoError(t, err)
	assert.Equal(t, sig, result)
}

func TestRequestContentSignature_ServerError_PlainText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(
			w,
			"get git ssh signature: failed to sign commit: exit status 1",
			http.StatusInternalServerError,
		)
	}))
	defer server.Close()

	signatureServerURL = server.URL + "/git-ssh-signature"
	t.Cleanup(func() { signatureServerURL = "" })

	_, err := requestContentSignature([]byte("commit content"), "/tmp/key.pub", log.Discard)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to sign commit")
	assert.NotContains(t, err.Error(), "invalid character")
}

func TestRequestContentSignature_ServerError_JSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "signing failed"})
	}))
	defer server.Close()

	signatureServerURL = server.URL + "/git-ssh-signature"
	t.Cleanup(func() { signatureServerURL = "" })

	_, err := requestContentSignature([]byte("commit content"), "/tmp/key.pub", log.Discard)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "signing failed")
}

func TestRequestContentSignature_Success(t *testing.T) {
	sig := []byte("-----BEGIN SSH SIGNATURE-----\ntest\n-----END SSH SIGNATURE-----")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(&GitSSHSignatureResponse{Signature: sig})
	}))
	defer server.Close()

	signatureServerURL = server.URL + "/git-ssh-signature"
	t.Cleanup(func() { signatureServerURL = "" })

	result, err := requestContentSignature([]byte("commit content"), "/tmp/key.pub", log.Discard)
	require.NoError(t, err)
	assert.Equal(t, sig, result)
	assert.Contains(t, string(result), "BEGIN SSH SIGNATURE")
}

func TestRequestContentSignature_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not json at all"))
	}))
	defer server.Close()

	signatureServerURL = server.URL + "/git-ssh-signature"
	t.Cleanup(func() { signatureServerURL = "" })

	_, err := requestContentSignature([]byte("commit content"), "/tmp/key.pub", log.Discard)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not json at all")
}
