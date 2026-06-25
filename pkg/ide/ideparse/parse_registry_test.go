package ideparse

import "testing"

func TestNewBrowserIDEsRegistered(t *testing.T) {
	// Assert a concrete options contract, not just existence: each new browser
	// IDE must expose a non-empty options map containing the stable VERSION
	// option, so a miswired or empty ide.Options map is caught instead of
	// silently passing.
	for _, name := range []string{"vscode-web", "code-server"} {
		opts, err := GetIDEOptions(name)
		if err != nil {
			t.Fatalf("GetIDEOptions(%q) error: %v", name, err)
		}
		if len(opts) == 0 {
			t.Fatalf("GetIDEOptions(%q) returned empty options", name)
		}
		if _, ok := opts["VERSION"]; !ok {
			t.Fatalf("GetIDEOptions(%q) missing expected VERSION option", name)
		}
	}
}
