package devcontainer

import (
	"encoding/json"
	"testing"

	"github.com/compose-spec/compose-go/v2/template"
	composetypes "github.com/compose-spec/compose-go/v2/types"
	"gopkg.in/yaml.v2"
)

// composeServiceName is the placeholder compose service used by these tests.
const composeServiceName = "app"

// composeLabelRoundTrip simulates what happens to a label value after devpod
// writes it into the generated docker-compose override file:
//  1. devpod escapes it (escapeComposeLabelValue) and yaml.Marshal's it.
//  2. docker compose parses that YAML back, recovering the escaped string.
//  3. docker compose performs variable interpolation ($$ -> $).
//
// The returned string is the value docker actually stores in the container's
// devcontainer.metadata label, i.e. what GetImageMetadataFromContainer reads.
func composeLabelRoundTrip(t *testing.T, raw string) string {
	t.Helper()

	svc := composetypes.ServiceConfig{
		Name: composeServiceName,
		Labels: composetypes.Labels{
			"devcontainer.metadata": escapeComposeLabelValue(raw),
		},
	}

	marshaled, err := yaml.Marshal(svc)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}

	var parsed composetypes.ServiceConfig
	if err := yaml.Unmarshal(marshaled, &parsed); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}

	noVars := func(string) (string, bool) { return "", false }
	interpolated, err := template.Substitute(parsed.Labels["devcontainer.metadata"], noVars)
	if err != nil {
		t.Fatalf("compose interpolation: %v", err)
	}
	return interpolated
}

// TestEscapeComposeLabelValue_PreservesQuoteHeavyMetadata is a regression test for
// customizations being silently dropped on Compose-based devcontainers.
//
// A quote-heavy lifecycle command (e.g. a postStartCommand using grep with a
// single-quoted regex) serializes into the devcontainer.metadata label as valid
// JSON containing single quotes. The label must survive the docker-compose
// override round-trip as valid JSON so customizations.vscode.extensions are not
// lost.
func TestEscapeComposeLabelValue_PreservesQuoteHeavyMetadata(t *testing.T) {
	// Exactly what json.Marshal of the merged metadata produces: a quote-heavy
	// postStartCommand plus a vscode customization. Valid JSON.
	raw := `[{"postStartCommand":"grep -qE '\"key\"\\s*:' file","customizations":` +
		`{"vscode":{"extensions":["golang.go"]}}}]`

	if !json.Valid([]byte(raw)) {
		t.Fatalf("precondition: raw metadata is not valid JSON")
	}

	got := composeLabelRoundTrip(t, raw)

	if !json.Valid([]byte(got)) {
		t.Fatalf("metadata label is not valid JSON after compose round-trip:\n%s", got)
	}

	var metadata []struct {
		PostStartCommand string `json:"postStartCommand"`
		Customizations   struct {
			VSCode struct {
				Extensions []string `json:"extensions"`
			} `json:"vscode"`
		} `json:"customizations"`
	}
	if err := json.Unmarshal([]byte(got), &metadata); err != nil {
		t.Fatalf("json.Unmarshal of round-tripped label failed: %v\nlabel: %s", err, got)
	}

	if len(metadata) != 1 {
		t.Fatalf("expected 1 metadata entry, got %d", len(metadata))
	}

	// The single quotes in the lifecycle command itself must survive the round-trip
	// byte-for-byte: this is the field that previously corrupted the whole label.
	const wantCmd = `grep -qE '"key"\s*:' file`
	if metadata[0].PostStartCommand != wantCmd {
		t.Fatalf("postStartCommand corrupted through round-trip:\n want: %s\n got:  %s",
			wantCmd, metadata[0].PostStartCommand)
	}

	exts := metadata[0].Customizations.VSCode.Extensions
	if len(exts) != 1 || exts[0] != "golang.go" {
		t.Fatalf("customizations.vscode.extensions lost: got %v", exts)
	}
}

// TestEscapeComposeLabelValue_EscapesDollarForCompose ensures we still prevent
// docker compose from interpolating literal '$' in label values (e.g. a command
// referencing $HOME), which is the legitimate purpose of the escaping.
func TestEscapeComposeLabelValue_EscapesDollarForCompose(t *testing.T) {
	raw := `[{"postStartCommand":"echo $HOME"}]`

	got := composeLabelRoundTrip(t, raw)

	if got != raw {
		t.Fatalf(
			"literal $ not preserved through compose round-trip:\n want: %s\n got:  %s",
			raw,
			got,
		)
	}
}
