package inject

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
)

type DownloadURLsTestSuite struct {
	suite.Suite
}

func (s *DownloadURLsTestSuite) TestNewDownloadURLs_WithoutTrailingSlash() {
	urls := NewDownloadURLs("https://github.com/skevetter/devpod/releases/latest/download")
	s.Equal("https://github.com/skevetter/devpod/releases/latest/download/devpod-linux-amd64", urls.Amd)
	s.Equal("https://github.com/skevetter/devpod/releases/latest/download/devpod-linux-arm64", urls.Arm)
}

func (s *DownloadURLsTestSuite) TestNewDownloadURLs_WithTrailingSlash() {
	urls := NewDownloadURLs("https://github.com/skevetter/devpod/releases/latest/download/")
	s.Equal("https://github.com/skevetter/devpod/releases/latest/download/devpod-linux-amd64", urls.Amd)
	s.Equal("https://github.com/skevetter/devpod/releases/latest/download/devpod-linux-arm64", urls.Arm)
}

func (s *DownloadURLsTestSuite) TestNewDownloadURLs_WithPlaceholder() {
	urls := NewDownloadURLs("https://example.com/releases/${BIN_NAME}")
	s.Equal("https://example.com/releases/devpod-linux-amd64", urls.Amd)
	s.Equal("https://example.com/releases/devpod-linux-arm64", urls.Arm)
}

func (s *DownloadURLsTestSuite) TestNoDoubleSlashes() {
	baseURLs := []string{
		"https://github.com/skevetter/devpod/releases/latest/download",
		"https://github.com/skevetter/devpod/releases/latest/download/",
		"https://github.com/skevetter/devpod/releases/latest/download//",
	}

	for _, baseURL := range baseURLs {
		urls := NewDownloadURLs(baseURL)
		s.False(s.containsDoubleSlash(urls.Amd), "AMD URL contains double slash: %s", urls.Amd)
		s.False(s.containsDoubleSlash(urls.Arm), "ARM URL contains double slash: %s", urls.Arm)
	}
}

func (s *DownloadURLsTestSuite) containsDoubleSlash(url string) bool {
	protocolEnd := s.findProtocolEnd(url)
	if protocolEnd == 0 {
		return false
	}
	pathPart := url[protocolEnd:]
	return strings.Contains(pathPart, "//")
}

func (s *DownloadURLsTestSuite) findProtocolEnd(url string) int {
	if idx := len("https://"); len(url) > idx && url[:idx] == "https://" {
		return idx
	}
	if idx := len("http://"); len(url) > idx && url[:idx] == "http://" {
		return idx
	}
	return 0
}

func TestDownloadURLsSuite(t *testing.T) {
	suite.Run(t, new(DownloadURLsTestSuite))
}
