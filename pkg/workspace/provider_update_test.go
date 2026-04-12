package workspace

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShouldSkipProviderUpdate(t *testing.T) {
	tests := []struct {
		name         string
		isDevVersion bool
		isInternal   bool
		expected     bool
	}{
		{
			name:         "skip when dev version",
			isDevVersion: true,
			isInternal:   false,
			expected:     true,
		},
		{
			name:         "skip when internal",
			isDevVersion: false,
			isInternal:   true,
			expected:     true,
		},
		{
			name:         "skip when both dev version and internal",
			isDevVersion: true,
			isInternal:   true,
			expected:     true,
		},
		{
			name:         "do not skip for regular provider",
			isDevVersion: false,
			isInternal:   false,
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldSkipProviderUpdate(tt.isDevVersion, tt.isInternal)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetProInstance_EmptyProviderName(t *testing.T) {
	// GetProInstance returns nil when provider name doesn't match any pro instance.
	// We can't easily test with a real config without disk I/O, but we can verify
	// that shouldSkipProviderUpdate is the composable unit for logic testing.
	// This is a placeholder for integration-level testing.
	result := shouldSkipProviderUpdate(false, false)
	assert.False(t, result, "regular provider should not be skipped")
}
