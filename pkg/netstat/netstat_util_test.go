package netstat

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type NetstatUtilTestSuite struct {
	suite.Suite
}

func TestNetstatUtilSuite(t *testing.T) {
	suite.Run(t, new(NetstatUtilTestSuite))
}

// TestDoNetstat_MissingFileReturnsEmpty verifies that a missing procfile (as
// seen on Linux hosts booted with ipv6.disable=1, where /proc/net/tcp6 does
// not exist) causes doNetstat to return an empty slice and nil error rather
// than propagating the os.IsNotExist error. This is the core regression
// guard for issue #705.
func (s *NetstatUtilTestSuite) TestDoNetstat_MissingFileReturnsEmpty() {
	missing := filepath.Join(s.T().TempDir(), "does-not-exist")

	entries, err := doNetstat(missing, NoopFilter)

	assert.NoError(s.T(), err, "missing procfile must not error")
	assert.Empty(s.T(), entries, "missing procfile must yield zero entries")
}

// TestDoNetstat_ParsesValidFile pins the happy-path behaviour of doNetstat so
// the IsNotExist fix cannot accidentally suppress successful parses. The
// synthetic content mimics the layout of /proc/net/tcp6: a header line
// followed by one listening IPv6 socket entry on [::]:22.
func (s *NetstatUtilTestSuite) TestDoNetstat_ParsesValidFile() {
	// Listening on [::]:22 (ssh). Field layout mirrors /proc/net/tcp6:
	// sl, local_address, rem_address, st, tx_queue:rx_queue, tr:tm->when,
	// retrnsmt, uid, timeout, inode, ... parseSocktab discards the header
	// and splits entries on whitespace, so single-spaced fields suffice.
	// Local address is 32 hex chars (ipv6StrLen) : 4 hex port (0x0016=22).
	// State 0x0A = Listen. UID is parsed as base-10.
	fields := []string{
		"0:",
		"00000000000000000000000000000000:0016",
		"00000000000000000000000000000000:0000",
		"0A",
		"00000000:00000000",
		"00:00000000",
		"00000000",
		"0",
		"0",
		"12345",
		"1",
		"0000000000000000",
		"100",
		"0",
		"0",
		"10",
		"0",
	}
	content := "header line discarded by parseSocktab\n" + strings.Join(fields, " ") + "\n"

	path := filepath.Join(s.T().TempDir(), "tcp6")
	require.NoError(s.T(), os.WriteFile(path, []byte(content), 0o600))

	entries, err := doNetstat(path, func(e *SockTabEntry) bool {
		return e.State == Listen
	})

	require.NoError(s.T(), err)
	require.Len(s.T(), entries, 1)
	assert.Equal(s.T(), uint16(22), entries[0].LocalAddr.Port)
	assert.Equal(s.T(), Listen, entries[0].State)
	assert.Equal(s.T(), uint32(0), entries[0].UID)
}

// TestDoNetstat_PropagatesNonNotExistOpenError verifies that the IsNotExist
// special case is surgical: other os.Open errors (here, permission denied)
// must still surface so real breakage is reported rather than silently
// swallowed. Skipped on Windows (different permission model) and when
// running as root (chmod 0 does not deny root on Linux).
func (s *NetstatUtilTestSuite) TestDoNetstat_PropagatesNonNotExistOpenError() {
	if runtime.GOOS == "windows" {
		s.T().Skip("permission semantics differ on Windows")
	}
	if os.Geteuid() == 0 {
		s.T().Skip("root bypasses file-mode permission checks")
	}

	path := filepath.Join(s.T().TempDir(), "unreadable")
	require.NoError(s.T(), os.WriteFile(path, []byte("irrelevant"), 0o000))
	s.T().Cleanup(func() { _ = os.Chmod(path, 0o600) })

	_, err := doNetstat(path, NoopFilter)

	require.Error(s.T(), err, "permission errors must propagate")
	assert.False(s.T(), os.IsNotExist(err), "error should not be IsNotExist")
}

// TestDoNetstat_PropagatesParseError ensures that once the file is
// successfully opened, malformed content still yields an error rather than
// being hidden by the IsNotExist fix.
func (s *NetstatUtilTestSuite) TestDoNetstat_PropagatesParseError() {
	path := filepath.Join(s.T().TempDir(), "garbage")
	require.NoError(s.T(), os.WriteFile(path, []byte("header\nnot enough fields here\n"), 0o600))

	_, err := doNetstat(path, NoopFilter)

	assert.Error(s.T(), err, "parser errors must propagate")
}
