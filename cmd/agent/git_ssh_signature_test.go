package agent

import (
	"testing"

	"github.com/skevetter/devpod/cmd/flags"
)

func TestGitSSHSignatureCmd_AcceptsUnknownFlags(t *testing.T) {
	cmd := NewGitSSHSignatureCmd(&flags.GlobalFlags{})

	// Simulate what git passes: -Y sign -n git -f /path/to/key -U /tmp/buffer
	// We expect flag parsing to succeed (no "unknown shorthand flag" error).
	err := cmd.ParseFlags(
		[]string{"-Y", "sign", "-n", "git", "-f", "/path/to/key", "-U", "/tmp/buffer"},
	)
	if err != nil {
		t.Fatalf("expected flag parsing to succeed with unknown flag -U, got: %v", err)
	}
}

func TestGitSSHSignatureCmd_KnownFlagsParsed(t *testing.T) {
	cmd := NewGitSSHSignatureCmd(&flags.GlobalFlags{})

	err := cmd.ParseFlags([]string{"-Y", "sign", "-n", "git", "-f", "/path/to/key", "/tmp/buffer"})
	if err != nil {
		t.Fatalf("expected flag parsing to succeed, got: %v", err)
	}

	val, err := cmd.Flags().GetString("command")
	if err != nil {
		t.Fatalf("expected to get 'command' flag, got: %v", err)
	}
	if val != "sign" {
		t.Fatalf("expected command flag to be 'sign', got: %q", val)
	}
}
