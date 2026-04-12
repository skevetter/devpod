package dotfiles

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractKeysFromEnvKeyValuePairs(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "empty input",
			input:    []string{},
			expected: []string{},
		},
		{
			name:     "single key-value pair",
			input:    []string{"FOO=bar"},
			expected: []string{"FOO"},
		},
		{
			name:     "multiple key-value pairs",
			input:    []string{"FOO=bar", "BAZ=qux"},
			expected: []string{"FOO", "BAZ"},
		},
		{
			name:     "value contains equals sign",
			input:    []string{"FOO=bar=baz"},
			expected: []string{"FOO"},
		},
		{
			name:     "entry without equals sign is skipped",
			input:    []string{"NOEQ", "FOO=bar"},
			expected: []string{"FOO"},
		},
		{
			name:     "empty value",
			input:    []string{"FOO="},
			expected: []string{"FOO"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractKeysFromEnvKeyValuePairs(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCollectDotfilesScriptEnvKeyValuePairs(t *testing.T) {
	t.Run("empty file list", func(t *testing.T) {
		result, err := collectDotfilesScriptEnvKeyValuePairs([]string{})
		assert.NoError(t, err)
		assert.Equal(t, []string{}, result)
	})

	t.Run("nonexistent file returns error", func(t *testing.T) {
		_, err := collectDotfilesScriptEnvKeyValuePairs([]string{"/nonexistent/file"})
		assert.Error(t, err)
	})
}

func TestBuildDotCmdAgentArguments(t *testing.T) {
	tests := []struct {
		name           string
		dotfilesRepo   string
		dotfilesScript string
		strictHostKey  bool
		debug          bool
		expected       []string
	}{
		{
			name:         "basic repo only",
			dotfilesRepo: "https://github.com/user/dotfiles",
			expected: []string{
				"agent", "workspace", "install-dotfiles",
				"--repository", "https://github.com/user/dotfiles",
			},
		},
		{
			name:           "with script",
			dotfilesRepo:   "https://github.com/user/dotfiles",
			dotfilesScript: "install.sh",
			expected: []string{
				"agent", "workspace", "install-dotfiles",
				"--repository", "https://github.com/user/dotfiles",
				"--install-script", "install.sh",
			},
		},
		{
			name:           "all options enabled",
			dotfilesRepo:   "https://github.com/user/dotfiles",
			dotfilesScript: "setup.sh",
			strictHostKey:  true,
			debug:          true,
			expected: []string{
				"agent", "workspace", "install-dotfiles",
				"--repository", "https://github.com/user/dotfiles",
				"--strict-host-key-checking", "--debug",
				"--install-script", "setup.sh",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildDotCmdAgentArguments(
				tt.dotfilesRepo,
				tt.dotfilesScript,
				tt.strictHostKey,
				tt.debug,
			)
			assert.Equal(t, tt.expected, result)
		})
	}
}
