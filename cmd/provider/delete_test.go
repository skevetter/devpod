package provider

import (
	"testing"

	"github.com/skevetter/devpod/cmd/flags"
)

func TestDeleteCmd_RejectsMultipleArgs(t *testing.T) {
	globalFlags := &flags.GlobalFlags{}
	cmd := NewDeleteCmd(globalFlags)
	err := cmd.Args(cmd, []string{"provider1", "provider2"})
	if err == nil {
		t.Fatal("expected error when passing multiple arguments to delete, got nil")
	}
}
