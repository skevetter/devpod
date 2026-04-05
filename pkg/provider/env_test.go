package provider

import (
	"slices"
	"strings"
	"testing"
)

func TestToEnvironment_ContainsProviderID(t *testing.T) {
	ws := &Workspace{
		ID:      "test-workspace",
		Context: "default",
		Provider: WorkspaceProviderConfig{
			Name: "test-provider",
		},
		Source: WorkspaceSource{},
	}

	environ := ToEnvironment(ws, nil, nil, nil)

	assertEnvContains(t, environ, "PROVIDER_ID", "test-provider")
	assertEnvContains(t, environ, "WORKSPACE_PROVIDER", "test-provider")
	assertEnvContains(t, environ, "WORKSPACE_ID", "test-workspace")
}

func TestToEnvironment_DoesNotDuplicateDevpodProvider(t *testing.T) {
	ws := &Workspace{
		ID:      "test-workspace",
		Context: "default",
		Provider: WorkspaceProviderConfig{
			Name: "test-provider",
		},
		Source: WorkspaceSource{},
	}

	environ := ToEnvironment(ws, nil, nil, nil)

	// DEVPOD_PROVIDER is reserved by the --provider CLI flag.
	// It may appear from os.Environ() but must not be explicitly added.
	count := 0
	for _, entry := range environ {
		if strings.HasPrefix(entry, "DEVPOD_PROVIDER=") {
			count++
		}
	}
	if count > 1 {
		t.Errorf(
			"found %d DEVPOD_PROVIDER entries; expected at most 1 (from os.Environ)",
			count,
		)
	}
}

func TestToEnvironment_IncludesExtraEnv(t *testing.T) {
	extra := map[string]string{"CUSTOM_VAR": "custom_value"}
	environ := ToEnvironment(nil, nil, nil, extra)

	assertEnvContains(t, environ, "CUSTOM_VAR", "custom_value")
}

func assertEnvContains(t *testing.T, environ []string, key, value string) {
	t.Helper()
	expected := key + "=" + value
	if !slices.Contains(environ, expected) {
		t.Errorf("expected %s in environment, not found", expected)
	}
}
