package codeserver

import (
	"strings"
	"testing"

	"github.com/skevetter/devpod/pkg/config"
)

func TestGetReleaseUrlDefaults(t *testing.T) {
	o := &CodeServerServer{values: nil}
	url := o.getReleaseUrl()
	const wantSubstr = "github.com/coder/code-server/releases/download/v4.126.0/code-server-4.126.0-linux-"
	if !strings.Contains(url, wantSubstr) {
		t.Fatalf("unexpected default url: %s", url)
	}
}

func TestGetReleaseUrlOverride(t *testing.T) {
	o := &CodeServerServer{values: map[string]config.OptionValue{
		DownloadAmd64Option: {Value: "https://example.com/amd64.tar.gz"},
		DownloadArm64Option: {Value: "https://example.com/arm64.tar.gz"},
	}}
	if !strings.HasPrefix(o.getReleaseUrl(), "https://example.com/") {
		t.Fatalf("override not used: %s", o.getReleaseUrl())
	}
}
