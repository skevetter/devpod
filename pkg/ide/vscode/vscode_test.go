package vscode

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/skevetter/log"
	"github.com/stretchr/testify/suite"
)

type DiscoverySuite struct {
	suite.Suite
	server *VsCodeServer
}

func TestDiscoverySuite(t *testing.T) {
	suite.Run(t, new(DiscoverySuite))
}

func (s *DiscoverySuite) SetupTest() {
	s.server = NewVSCodeServer(ServerOptions{
		Flavor: FlavorStable,
		Log:    log.Discard,
	})
}

func (s *DiscoverySuite) TestMatchServerProcess_MatchesBinaryName() {
	cmdline := []byte(
		"/home/user/.vscode-server/cli/servers/Stable-abc123/server/bin/code-server\x00--host\x000.0.0.0",
	)
	result := matchServerProcess(cmdline, "code-server")
	s.Contains(result, "code-server")
	s.Contains(result, ".vscode-server")
}

func (s *DiscoverySuite) TestMatchServerProcess_NoMatchDifferentBinary() {
	cmdline := []byte("/usr/bin/bash\x00-c\x00echo hello")
	result := matchServerProcess(cmdline, "code-server")
	s.Empty(result)
}

func (s *DiscoverySuite) TestMatchServerProcess_MatchesShInvocation() {
	cmdline := []byte(
		"/bin/sh\x00/home/user/.vscode-server/bin/code-server\x00--host\x000.0.0.0",
	)
	result := matchServerProcess(cmdline, "code-server")
	s.Contains(result, "code-server")
}

func (s *DiscoverySuite) TestMatchServerProcess_EmptyCmdline() {
	result := matchServerProcess([]byte{}, "code-server")
	s.Empty(result)
}

func (s *DiscoverySuite) TestMatchServerProcess_CursorFlavor() {
	cmdline := []byte(
		"/home/user/.cursor-server/cli/servers/Stable-xyz/server/bin/cursor-server\x00--host\x000.0.0.0",
	)
	result := matchServerProcess(cmdline, "cursor-server")
	s.Contains(result, "cursor-server")
}

func (s *DiscoverySuite) TestMatchServerProcess_DoesNotCrossMatch() {
	cmdline := []byte("/home/user/.vscode-server/bin/code-server\x00--host\x000.0.0.0")
	result := matchServerProcess(cmdline, "cursor-server")
	s.Empty(result)
}

func (s *DiscoverySuite) TestIsNumeric() {
	s.True(isNumeric("123"))
	s.True(isNumeric("1"))
	s.False(isNumeric(""))
	s.False(isNumeric("abc"))
	s.False(isNumeric("12a"))
}

func (s *DiscoverySuite) TestFindInDir_SingleBinary() {
	root := s.T().TempDir()
	binDir := filepath.Join(root, "cli", "servers", "Stable-abc", "server", "bin")
	binPath := s.createTestServerBinary(binDir)

	result := s.server.findInDir(root, "code-server")
	s.Equal(binPath, result)
}

func (s *DiscoverySuite) TestFindInDir_PrefersNewestDirectory() {
	root := s.T().TempDir()

	// Create an older server version
	oldDir := filepath.Join(root, "cli", "servers", "Stable-old", "server", "bin")
	s.createTestServerBinary(oldDir)
	oldTime := time.Now().Add(-24 * time.Hour)
	s.Require().NoError(os.Chtimes(oldDir, oldTime, oldTime))

	// Create a newer server version
	newDir := filepath.Join(root, "cli", "servers", "Stable-new", "server", "bin")
	newBin := s.createTestServerBinary(newDir)

	result := s.server.findInDir(root, "code-server")
	s.Equal(newBin, result)
}

func (s *DiscoverySuite) TestFindInDir_SkipsStagingDirectory() {
	root := s.T().TempDir()
	stagingDir := filepath.Join(root, "cli", "servers", "Stable-abc.staging", "server", "bin")
	s.createTestServerBinary(stagingDir)

	result := s.server.findInDir(root, "code-server")
	s.Empty(result)
}

func (s *DiscoverySuite) TestFindInDir_NoBinaries() {
	root := s.T().TempDir()
	result := s.server.findInDir(root, "code-server")
	s.Empty(result)
}

func (s *DiscoverySuite) createTestServerBinary(dir string) string {
	s.T().Helper()
	// #nosec G301 -- test helper needs dirs traversable by the walker
	s.Require().NoError(os.MkdirAll(dir, 0o750))
	binPath := filepath.Join(dir, "code-server")
	s.Require().NoError(os.WriteFile(binPath, []byte("#!/bin/sh\n"), 0o600))
	return binPath
}
