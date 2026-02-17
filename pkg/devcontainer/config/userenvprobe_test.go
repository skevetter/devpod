package config

import (
	"reflect"
	"testing"

	"github.com/skevetter/log"
)

//nolint:funlen,lll // It's just test input vectors.
func TestParseProbeOutput(t *testing.T) {
	tests := []struct {
		name     string
		output   []byte
		sep      byte
		expected map[string]string
	}{
		{
			name:   "simple environment variables with null separator",
			output: []byte("PATH=/usr/bin\x00HOME=/home/user\x00USER=testuser\x00"),
			sep:    '\x00',
			expected: map[string]string{
				"PATH": "/usr/bin",
				"HOME": "/home/user",
				"USER": "testuser",
			},
		},
		{
			name:   "environment variable with multiple equals signs",
			output: []byte("SIMPLE=value\x00COMPLEX=key=value=more\x00"),
			sep:    '\x00',
			expected: map[string]string{
				"SIMPLE":  "value",
				"COMPLEX": "key=value=more",
			},
		},
		{
			name:   "bash function with newlines and special characters",
			output: []byte("BASH_FUNC_scl%%=() {  if [ \"$1\" = \"load\" -o \"$1\" = \"unload\" ]; then\n eval \"module $@\";\n else\n /usr/bin/scl \"$@\";\n fi\n}\x00PATH=/usr/bin\x00"),
			sep:    '\x00',
			expected: map[string]string{
				"BASH_FUNC_scl%%": "() {  if [ \"$1\" = \"load\" -o \"$1\" = \"unload\" ]; then\n eval \"module $@\";\n else\n /usr/bin/scl \"$@\";\n fi\n}",
				"PATH":            "/usr/bin",
			},
		},
		{
			name:   "environment variable with leading and trailing whitespace",
			output: []byte("VAR1=  value with spaces  \x00VAR2=\tvalue with tab\t\x00"),
			sep:    '\x00',
			expected: map[string]string{
				"VAR1": "  value with spaces  ",
				"VAR2": "\tvalue with tab\t",
			},
		},
		{
			name:   "environment variable with newlines in value",
			output: []byte("MULTILINE=line1\nline2\nline3\x00SIMPLE=test\x00"),
			sep:    '\x00',
			expected: map[string]string{
				"MULTILINE": "line1\nline2\nline3",
				"SIMPLE":    "test",
			},
		},
		{
			name:     "empty output",
			output:   []byte(""),
			sep:      '\x00',
			expected: map[string]string{},
		},
		{
			name:   "environment variable with empty value",
			output: []byte("EMPTY=\x00NONEMPTY=value\x00"),
			sep:    '\x00',
			expected: map[string]string{
				"EMPTY":    "",
				"NONEMPTY": "value",
			},
		},
		{
			name:   "invalid entry without equals sign",
			output: []byte("INVALID\x00VALID=value\x00"),
			sep:    '\x00',
			expected: map[string]string{
				"VALID": "value",
			},
		},
		{
			name:   "newline separator (printenv format)",
			output: []byte("PATH=/usr/bin\nHOME=/home/user\nUSER=testuser\n"),
			sep:    '\n',
			expected: map[string]string{
				"PATH": "/usr/bin",
				"HOME": "/home/user",
				"USER": "testuser",
			},
		},
		{
			name:   "complex bash function with multiple newlines",
			output: []byte("BASH_FUNC_module%%=() {\n eval `/usr/bin/modulecmd bash $*`\n}\x00SHELL=/bin/bash\x00"),
			sep:    '\x00',
			expected: map[string]string{
				"BASH_FUNC_module%%": "() {\n eval `/usr/bin/modulecmd bash $*`\n}",
				"SHELL":              "/bin/bash",
			},
		},
		{
			name:   "environment variable with equals in value at start",
			output: []byte("VAR==value\x00"),
			sep:    '\x00',
			expected: map[string]string{
				"VAR": "=value",
			},
		},
		{
			name:   "multiple consecutive equals signs",
			output: []byte("VAR1===\x00VAR2=a=b=c=d\x00"),
			sep:    '\x00',
			expected: map[string]string{
				"VAR1": "==",
				"VAR2": "a=b=c=d",
			},
		},
		{
			name:   "bash function from RHEL",
			output: []byte("BASH_FUNC_scl%%=() {  if [ \"$1\" = \"load\" -o \"$1\" = \"unload\" ]; then\n eval \"module $@\";\n else\n /usr/bin/scl \"$@\";\n fi\n}\x00"),
			sep:    '\x00',
			expected: map[string]string{
				"BASH_FUNC_scl%%": "() {  if [ \"$1\" = \"load\" -o \"$1\" = \"unload\" ]; then\n eval \"module $@\";\n else\n /usr/bin/scl \"$@\";\n fi\n}",
			},
		},
		{
			name:     "trailing separator only",
			output:   []byte("\x00"),
			sep:      '\x00',
			expected: map[string]string{},
		},
		{
			name:   "multiple trailing separators",
			output: []byte("VAR=value\x00\x00\x00"),
			sep:    '\x00',
			expected: map[string]string{
				"VAR": "value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testLogger := log.Default.ErrorStreamOnly()
			result := parseProbeOutput(tt.output, tt.sep, testLogger)

			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("parseProbeOutput() mismatch\ngot:  %+v\nwant: %+v", result, tt.expected)
			}
		})
	}
}
