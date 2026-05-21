package config

import "testing"

func TestParseMount_PreservesEqualsInValue(t *testing.T) {
	mount := ParseMount("type=volume,source=my-cache=1,target=/cache")

	if mount.Type != "volume" {
		t.Fatalf("expected type volume, got %q", mount.Type)
	}
	if mount.Source != "my-cache=1" {
		t.Fatalf("expected source my-cache=1, got %q", mount.Source)
	}
	if mount.Target != "/cache" {
		t.Fatalf("expected target /cache, got %q", mount.Target)
	}
}

func TestParseMount_InvalidSegmentDoesNotPanic(t *testing.T) {
	mount := ParseMount("type=volume,readonly,target=/cache")

	if len(mount.Other) != 1 || mount.Other[0] != "readonly" {
		t.Fatalf("expected readonly in other, got %#v", mount.Other)
	}
}
