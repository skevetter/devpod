package gitsshsigning

import (
	"encoding/json"
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
