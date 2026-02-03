package dockercredentials

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skevetter/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteCredentialHelperDockerless(t *testing.T) {
	tempDir := t.TempDir()
	logger := log.Default

	err := writeCredentialHelperDockerless(tempDir, 12345, logger)
	require.NoError(t, err)

	helperPath := filepath.Join(tempDir, getCredentialHelperFilename())
	content, err := os.ReadFile(helperPath)
	require.NoError(t, err)

	contentStr := string(content)

	assert.True(t, strings.HasPrefix(contentStr, "#!/.dockerless/bin/sh\n"),
		"dockerless credential helper should use #!/.dockerless/bin/sh shebang")

	assert.Contains(t, contentStr, "agent docker-credentials --port 12345")

	info, err := os.Stat(helperPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0755), info.Mode().Perm())
}

func TestWriteCredentialHelper(t *testing.T) {
	tempDir := t.TempDir()
	logger := log.Default

	err := writeCredentialHelper(tempDir, 12345, logger)
	require.NoError(t, err)

	helperPath := filepath.Join(tempDir, getCredentialHelperFilename())
	content, err := os.ReadFile(helperPath)
	require.NoError(t, err)

	contentStr := string(content)

	assert.True(t, strings.HasPrefix(contentStr, "#!/bin/sh\n"),
		"standard credential helper should use #!/bin/sh shebang")

	assert.Contains(t, contentStr, "agent docker-credentials --port 12345")

	info, err := os.Stat(helperPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0755), info.Mode().Perm())
}

func TestConfigureCredentialsDockerless(t *testing.T) {
	tempDir := t.TempDir()
	logger := log.Default

	origDockerConfig := os.Getenv("DOCKER_CONFIG")
	origPath := os.Getenv("PATH")
	t.Cleanup(func() {
		os.Setenv("DOCKER_CONFIG", origDockerConfig)
		os.Setenv("PATH", origPath)
	})

	dockerConfigDir, err := ConfigureCredentialsDockerless(tempDir, 12345, logger)
	require.NoError(t, err)

	_, err = os.Stat(dockerConfigDir)
	require.NoError(t, err)

	helperPath := filepath.Join(dockerConfigDir, getCredentialHelperFilename())
	content, err := os.ReadFile(helperPath)
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(string(content), "#!/.dockerless/bin/sh\n"),
		"ConfigureCredentialsDockerless should create helper with dockerless shebang")

	assert.Equal(t, dockerConfigDir, os.Getenv("DOCKER_CONFIG"))

	assert.Contains(t, os.Getenv("PATH"), dockerConfigDir)
}
