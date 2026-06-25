package ideparse

import "testing"

func TestNewBrowserIDEsRegistered(t *testing.T) {
	for _, name := range []string{"vscode-web", "code-server"} {
		opts, err := GetIDEOptions(name)
		if err != nil {
			t.Fatalf("GetIDEOptions(%q) error: %v", name, err)
		}
		if opts == nil {
			t.Fatalf("GetIDEOptions(%q) returned nil options", name)
		}
	}
}
