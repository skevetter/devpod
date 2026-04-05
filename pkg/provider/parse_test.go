package provider

import (
	"testing"

	"github.com/skevetter/devpod/pkg/types"
)

func TestValidate_RejectsReservedOptionNames(t *testing.T) {
	reserved := []string{
		"PROVIDER_ID",
		"PROVIDER_CONTEXT",
		"PROVIDER_FOLDER",
		"WORKSPACE_ID",
		"WORKSPACE_UID",
		"WORKSPACE_PROVIDER",
		"WORKSPACE_CONTEXT",
		"WORKSPACE_FOLDER",
		"WORKSPACE_SOURCE",
		"WORKSPACE_ORIGIN",
		"WORKSPACE_PICTURE",
		"MACHINE_ID",
		"MACHINE_CONTEXT",
		"MACHINE_FOLDER",
		"MACHINE_PROVIDER",
		"LOFT_PROJECT",
	}

	for _, name := range reserved {
		cfg := &ProviderConfig{
			Name: "test-provider",
			Options: map[string]*types.Option{
				name: {Description: "test"},
			},
		}
		err := validate(cfg)
		if err == nil {
			t.Errorf("expected error for reserved option name %q, got nil", name)
		}
	}
}

func TestValidate_AllowsNonReservedOptionNames(t *testing.T) {
	cfg := &ProviderConfig{
		Name: "test-provider",
		Options: map[string]*types.Option{
			"MY_CUSTOM_OPTION": {Description: "test"},
			"AWS_REGION":       {Description: "test"},
		},
		Exec: ProviderCommands{
			Command: []string{"echo hello"},
		},
	}
	err := validate(cfg)
	if err != nil {
		t.Errorf("expected no error for non-reserved names, got: %v", err)
	}
}
