package dockerinstall

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
)

type DetectorTestSuite struct {
	suite.Suite
	detector *Detector
}

func (s *DetectorTestSuite) SetupTest() {
	s.detector = NewDetector()
}

func TestDetectorSuite(t *testing.T) {
	suite.Run(t, new(DetectorTestSuite))
}

func (s *DetectorTestSuite) TestParseOSRelease_Ubuntu_WithCodename() {
	osRelease := `NAME="Ubuntu"
VERSION="22.04.1 LTS (Jammy Jellyfish)"
ID=ubuntu
ID_LIKE=debian
VERSION_ID="22.04"
VERSION_CODENAME=jammy`

	distro := s.detector.parseOSRelease(strings.NewReader(osRelease))
	s.Equal("ubuntu", distro.ID)
	s.Equal("jammy", distro.Version)
}

func (s *DetectorTestSuite) TestParseOSRelease_Ubuntu_WithoutCodename() {
	osRelease := `NAME="Ubuntu"
VERSION="22.04.1 LTS"
ID=ubuntu
ID_LIKE=debian
VERSION_ID="22.04"`

	distro := s.detector.parseOSRelease(strings.NewReader(osRelease))
	s.Equal("ubuntu", distro.ID)
	s.Equal("22.04", distro.Version, "Should fall back to VERSION_ID when codename missing")
}

func (s *DetectorTestSuite) TestParseOSRelease_Debian_WithCodename() {
	osRelease := `NAME="Debian GNU/Linux"
VERSION="12 (bookworm)"
ID=debian
VERSION_ID="12"
VERSION_CODENAME=bookworm`

	distro := s.detector.parseOSRelease(strings.NewReader(osRelease))
	s.Equal("debian", distro.ID)
	s.Equal("bookworm", distro.Version)
}

func (s *DetectorTestSuite) TestParseOSRelease_Debian() {
	// Real-world example from Debian 13 (trixie)
	osRelease := `PRETTY_NAME="Debian GNU/Linux 13 (trixie)"
NAME="Debian GNU/Linux"
VERSION_ID="13"
VERSION="13 (trixie)"
VERSION_CODENAME=trixie
DEBIAN_VERSION_FULL=13.2
ID=debian
HOME_URL="https://www.debian.org/"
SUPPORT_URL="https://www.debian.org/support"
BUG_REPORT_URL="https://bugs.debian.org/"`

	distro := s.detector.parseOSRelease(strings.NewReader(osRelease))
	s.Equal("debian", distro.ID)
	s.Equal("trixie", distro.Version, "Should extract VERSION_CODENAME, not VERSION_ID")
	s.False(isNumericVersion(distro.Version), "Version should be codename, not numeric")
}

func (s *DetectorTestSuite) TestParseOSRelease_Debian_WithoutCodename() {
	osRelease := `NAME="Debian GNU/Linux"
VERSION="12"
ID=debian
VERSION_ID="12"`

	distro := s.detector.parseOSRelease(strings.NewReader(osRelease))
	s.Equal("debian", distro.ID)
	s.Equal("12", distro.Version, "Should fall back to VERSION_ID")
}

func (s *DetectorTestSuite) TestParseOSRelease_Fedora() {
	osRelease := `NAME="Fedora Linux"
VERSION="39 (Workstation Edition)"
ID=fedora
VERSION_ID=39`

	distro := s.detector.parseOSRelease(strings.NewReader(osRelease))
	s.Equal("fedora", distro.ID)
	s.Equal("39", distro.Version)
}

func (s *DetectorTestSuite) TestParseOSRelease_QuotedValues() {
	osRelease := `ID="ubuntu"
VERSION_CODENAME="jammy"`

	distro := s.detector.parseOSRelease(strings.NewReader(osRelease))
	s.Equal("ubuntu", distro.ID)
	s.Equal("jammy", distro.Version)
}

func (s *DetectorTestSuite) TestParseOSRelease_UnquotedValues() {
	osRelease := `ID=ubuntu
VERSION_CODENAME=jammy`

	distro := s.detector.parseOSRelease(strings.NewReader(osRelease))
	s.Equal("ubuntu", distro.ID)
	s.Equal("jammy", distro.Version)
}

func (s *DetectorTestSuite) TestParseOSRelease_Empty() {
	distro := s.detector.parseOSRelease(strings.NewReader(""))
	s.Equal("", distro.ID)
	s.Equal("", distro.Version)
}

func (s *DetectorTestSuite) TestMapDebianVersion() {
	tests := []struct {
		input    string
		expected string
	}{
		{"13\n", "trixie"},
		{"12\n", "bookworm"},
		{"11\n", "bullseye"},
		{"10\n", "buster"},
		{"9\n", "stretch"},
		{"8\n", "jessie"},
		{"12.5\n", "bookworm"},
		{"11/sid\n", "bullseye"},
		{"99\n", ""},
		{"invalid\n", ""},
	}

	for _, tt := range tests {
		result := s.detector.mapDebianVersion([]byte(tt.input))
		s.Equal(tt.expected, result, "Input: %s", tt.input)
	}
}

func (s *DetectorTestSuite) TestMapDebianID() {
	s.Equal("raspbian", s.detector.mapDebianID("osmc"))
	s.Equal("debian", s.detector.mapDebianID("linuxmint"))
	s.Equal("debian", s.detector.mapDebianID("unknown"))
}

func (s *DetectorTestSuite) TestIsNumericVersion() {
	tests := []struct {
		version  string
		expected bool
	}{
		{"22.04", true},
		{"11", true},
		{"12.5", true},
		{"jammy", false},
		{"bookworm", false},
		{"focal", false},
		{"", false},
		{"1.2.3", true},
		{"v22.04", false},
		{"22-04", false},
	}

	for _, tt := range tests {
		result := isNumericVersion(tt.version)
		s.Equal(tt.expected, result, "Version: %s", tt.version)
	}
}

func (s *DetectorTestSuite) TestDetectOS() {
	os := s.detector.DetectOS()
	s.NotEmpty(os)
	s.Contains([]string{"linux", "darwin", "windows"}, os)
}

func (s *DetectorTestSuite) TestDistro_HasCodename() {
	tests := []struct {
		distro   *Distro
		expected bool
	}{
		{&Distro{ID: "ubuntu", Version: "jammy"}, true},
		{&Distro{ID: "debian", Version: "bookworm"}, true},
		{&Distro{ID: "ubuntu", Version: "22.04"}, false},
		{&Distro{ID: "debian", Version: "12"}, false},
		{&Distro{ID: "fedora", Version: "39"}, false},
		{&Distro{ID: "ubuntu", Version: ""}, false},
	}

	for _, tt := range tests {
		result := tt.distro.HasCodename()
		s.Equal(tt.expected, result, "Distro: %s %s", tt.distro.ID, tt.distro.Version)
	}
}
