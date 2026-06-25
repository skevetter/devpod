package vscodeweb

import (
	"strings"
	"testing"

	"github.com/skevetter/devpod/pkg/config"
)

func TestGetReleaseUrlDefaults(t *testing.T) {
	o := &VSCodeWebServer{values: nil}
	url := o.getReleaseUrl()
	if !strings.HasPrefix(
		url,
		"https://code.visualstudio.com/sha/download?build=stable&os=cli-alpine-",
	) {
		t.Fatalf("unexpected default url: %s", url)
	}
}

func TestGetReleaseUrlOverride(t *testing.T) {
	o := &VSCodeWebServer{values: map[string]config.OptionValue{
		DownloadAmd64Option: {Value: "https://example.com/amd64.tar.gz"},
		DownloadArm64Option: {Value: "https://example.com/arm64.tar.gz"},
	}}
	url := o.getReleaseUrl()
	if !strings.HasPrefix(url, "https://example.com/") {
		t.Fatalf("override not used: %s", url)
	}
}
