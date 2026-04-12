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

func TestProviderVersionNeedsUpdate(t *testing.T) {
	tests := []struct {
		name, newVer, curVer string
		expected, expectErr  bool
	}{
		{"same version", "v0.5.0", "v0.5.0", false, false},
		{"newer version", "v0.6.0", "v0.5.0", true, false},
		{"older version (downgrade)", "v0.4.0", "v0.5.0", true, false},
		{"mixed v prefix", "v0.6.0", "0.5.0", true, false},
		{"patch difference", "v1.2.4", "v1.2.3", true, false},
		{"invalid new version", "not-a-version", "v0.5.0", false, true},
		{"invalid current version", "v0.5.0", "not-a-version", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := providerVersionNeedsUpdate(tt.newVer, tt.curVer)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
