# Maintained Browser VS Code IDEs Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add two actively-maintained browser VS Code IDEs — `vscode-web` (VS Code CLI `code serve-web`) and `code-server` (Coder) — alongside the unmaintained `openvscode`, fully wired into the CLI and the desktop app.

**Architecture:** Two self-contained Go packages mirror `pkg/ide/openvscode/` (`Install`/`Start`/`InstallExtensions`/`Options`/`getReleaseUrl`). Each is registered in the IDE registry, the agent container setup switch, an async-extension command, and the browser opener — exactly how `openvscode` is wired. The desktop app gets both IDEs as experimental entries (default visible), mirroring the IBM Bob PR (#657).

**Tech Stack:** Go (agent + CLI), React/TypeScript + Tauri (desktop), Ginkgo (e2e).

## Global Constraints

- IDE name strings (the `--ide=` values, semi-permanent): `vscode-web` and `code-server`.
- Go package dirs (no hyphens allowed): `pkg/ide/vscodeweb/` and `pkg/ide/codeserver/`.
- Install dirs MUST be distinct from `~/.vscode-server` (the regular VS Code server, whose binary is itself named `code-server`): use `~/.vscode-web` and `~/.code-server`.
- `code serve-web` MUST be started with `--accept-server-license-terms` (non-interactive) and `--without-connection-token`.
- Both IDEs: `Experimental: true`, `Group: config.IDEGroupPrimary`, default toggle `true` (visible).
- code-server default `VERSION`: `4.126.0`. vscode-web default `VERSION` (build channel): `stable`.
- Reuse existing helpers — do not reinvent: `vscode.InstallAPKRequirements`, `command.StartBackgroundOnce`, `command.GetHome`, `extract.Extract`/`extract.StripLevels`, `copy2.ChownR`, `devpodhttp.GetHTTPClient`, `util.UserHomeDir`, `tunnel.StartBrowserTunnel`, `open2.Open`, `ParseAddressAndPort`.
- Do NOT edit `desktop/src-tauri/src/settings.rs` or run the ts-rs generator: the struct is `Serialize`-only and already drifted (missing `experimental_bob`); regenerating would drop `bob`. Hand-edit `desktop/src/gen/Settings.ts` directly, exactly as PR #657 did.
- Conventional commit messages. Commit after each task.

---

### Task 1: `vscode-web` Go package (VS Code CLI `serve-web`)

**Files:**
- Modify: `pkg/config/ide.go` (add IDE constant)
- Create: `pkg/ide/vscodeweb/vscodeweb.go`
- Test: `pkg/ide/vscodeweb/vscodeweb_test.go`

**Interfaces:**
- Produces: package `vscodeweb` with `Options ide.Options`, `DefaultVSCodePort = 10800`, `func NewVSCodeWebServer(extensions []string, settings string, userName string, host, port string, values map[string]config.OptionValue, log log.Logger) *VSCodeWebServer`, and methods `Install() error`, `Start() error`, `InstallExtensions() error`. Option key consts `BindAddressOption`, `OpenOption`, `ForwardPortsOption`. Config const `config.IDEVSCodeWeb = "vscode-web"`.

- [ ] **Step 1: Add the config constant**

In `pkg/config/ide.go`, add the constant directly after `IDEOpenVSCode`:

```go
	IDEOpenVSCode      IDE = "openvscode"
	IDEVSCodeWeb       IDE = "vscode-web"
	IDECodeServer      IDE = "code-server"
```

(Both constants are added now so later tasks compile; `code-server`'s package arrives in Task 2.)

- [ ] **Step 2: Write the failing test**

Create `pkg/ide/vscodeweb/vscodeweb_test.go`:

```go
package vscodeweb

import (
	"strings"
	"testing"
)

func TestGetReleaseUrlDefaults(t *testing.T) {
	o := &VSCodeWebServer{values: nil}
	url := o.getReleaseUrl()
	if !strings.HasPrefix(url, "https://code.visualstudio.com/sha/download?build=stable&os=cli-alpine-") {
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
```

Add the import `"github.com/skevetter/devpod/pkg/config"` to the test file.

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./pkg/ide/vscodeweb/...`
Expected: FAIL — package/`VSCodeWebServer` undefined.

- [ ] **Step 4: Write the implementation**

Create `pkg/ide/vscodeweb/vscodeweb.go`:

```go
package vscodeweb

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/pkg/command"
	"github.com/skevetter/devpod/pkg/config"
	copy2 "github.com/skevetter/devpod/pkg/copy"
	"github.com/skevetter/devpod/pkg/extract"
	devpodhttp "github.com/skevetter/devpod/pkg/http"
	"github.com/skevetter/devpod/pkg/ide"
	"github.com/skevetter/devpod/pkg/ide/vscode"
	"github.com/skevetter/devpod/pkg/util"
	"github.com/skevetter/log"
)

const (
	// The VS Code CLI ("code" binary) is a small static musl build that runs
	// "code serve-web". The %s is the build channel (stable | insider).
	DownloadAmd64Template = "https://code.visualstudio.com/sha/download?build=%s&os=cli-alpine-x64"
	DownloadArm64Template = "https://code.visualstudio.com/sha/download?build=%s&os=cli-alpine-arm64"
)

const (
	ForwardPortsOption  = "FORWARD_PORTS"
	OpenOption          = "OPEN"
	BindAddressOption   = "BIND_ADDRESS"
	VersionOption       = "VERSION"
	DownloadAmd64Option = "DOWNLOAD_AMD64"
	DownloadArm64Option = "DOWNLOAD_ARM64"
)

var Options = ide.Options{
	ForwardPortsOption: {
		Name:        ForwardPortsOption,
		Description: "If DevPod should automatically do port-forwarding",
		Default:     "true",
		Enum:        []string{"true", "false"},
	},
	BindAddressOption: {
		Name:        BindAddressOption,
		Description: "The address to bind VS Code Web to locally, e.g. 0.0.0.0:12345",
		Default:     "",
	},
	VersionOption: {
		Name:        VersionOption,
		Description: "The VS Code CLI build channel to download (stable or insider)",
		Default:     "stable",
	},
	OpenOption: {
		Name:        OpenOption,
		Description: "If DevPod should automatically open the browser",
		Default:     "true",
		Enum:        []string{"true", "false"},
	},
	DownloadArm64Option: {
		Name:        DownloadArm64Option,
		Description: "The download url for the arm64 VS Code CLI binary",
	},
	DownloadAmd64Option: {
		Name:        DownloadAmd64Option,
		Description: "The download url for the amd64 VS Code CLI binary",
	},
}

const DefaultVSCodePort = 10800

func NewVSCodeWebServer(
	extensions []string,
	settings string,
	userName string,
	host, port string,
	values map[string]config.OptionValue,
	log log.Logger,
) *VSCodeWebServer {
	return &VSCodeWebServer{
		values:     values,
		extensions: extensions,
		settings:   settings,
		userName:   userName,
		host:       host,
		port:       port,
		log:        log,
	}
}

type VSCodeWebServer struct {
	values     map[string]config.OptionValue
	extensions []string
	settings   string
	userName   string
	host       string
	port       string
	log        log.Logger
}

func (o *VSCodeWebServer) InstallExtensions() error {
	if err := o.installExtensions(); err != nil {
		return fmt.Errorf("install extensions: %w", err)
	}
	return nil
}

func (o *VSCodeWebServer) Install() error {
	location, err := prepareVSCodeWebLocation(o.userName)
	if err != nil {
		return err
	}

	// already installed?
	if _, err = os.Stat(filepath.Join(location, "code")); err == nil {
		return nil
	}

	url := o.getReleaseUrl()

	vscode.InstallAPKRequirements(o.log)

	resp, err := devpodhttp.GetHTTPClient().Get(url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	// The CLI tarball is a single flat "code" file; StripLevels(1) keeps it
	// (the strip loop stops on single-component names).
	if err = extract.Extract(resp.Body, location, extract.StripLevels(1)); err != nil {
		return fmt.Errorf("extract vscode cli: %w", err)
	}

	if o.userName != "" {
		if err = copy2.ChownR(location, o.userName); err != nil {
			return fmt.Errorf("chown: %w", err)
		}
	}

	if err = o.installSettings(); err != nil {
		return fmt.Errorf("install settings: %w", err)
	}

	return nil
}

func (o *VSCodeWebServer) getReleaseUrl() string {
	var url string
	version := Options.GetValue(o.values, VersionOption)

	if runtime.GOARCH == "arm64" {
		url = Options.GetValue(o.values, DownloadArm64Option)
		if url == "" {
			url = fmt.Sprintf(DownloadArm64Template, version)
		}
	} else {
		url = Options.GetValue(o.values, DownloadAmd64Option)
		if url == "" {
			url = fmt.Sprintf(DownloadAmd64Template, version)
		}
	}

	return url
}

func (o *VSCodeWebServer) installExtensions() error {
	if len(o.extensions) == 0 {
		return nil
	}

	location, err := prepareVSCodeWebLocation(o.userName)
	if err != nil {
		return err
	}

	out := o.log.Writer(logrus.InfoLevel, false)
	defer func() { _ = out.Close() }()

	binaryPath := filepath.Join(location, "code")
	extensionsDir := filepath.Join(location, "extensions")
	for _, extension := range o.extensions {
		o.log.Info("Install extension " + extension + "...")
		runCommand := fmt.Sprintf(
			"%s --install-extension '%s' --extensions-dir '%s'",
			binaryPath, extension, extensionsDir,
		)
		args := []string{}
		if o.userName != "" {
			args = append(args, "su", o.userName, "-c", runCommand)
		} else {
			args = append(args, "sh", "-c", runCommand)
		}
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdout = out
		cmd.Stderr = out
		if err = cmd.Run(); err != nil {
			o.log.Errorf("failed installing extension: extension=%s, error=%v", extension, err)
		} else {
			o.log.Infof("installed extension: extension=%s", extension)
		}
	}

	return nil
}

func (o *VSCodeWebServer) installSettings() error {
	if len(o.settings) == 0 {
		return nil
	}

	location, err := prepareVSCodeWebLocation(o.userName)
	if err != nil {
		return err
	}

	settingsDir := filepath.Join(location, "data", "Machine")
	// #nosec G301
	if err = os.MkdirAll(settingsDir, 0o755); err != nil {
		return err
	}

	if err = os.WriteFile(filepath.Join(settingsDir, "settings.json"), []byte(o.settings), 0o600); err != nil {
		return err
	}

	return copy2.ChownR(location, o.userName)
}

func (o *VSCodeWebServer) Start() error {
	location, err := prepareVSCodeWebLocation(o.userName)
	if err != nil {
		return err
	}

	if o.host == "" {
		o.host = "0.0.0.0"
	}
	if o.port == "" {
		o.port = strconv.Itoa(DefaultVSCodePort)
	}

	binaryPath := filepath.Join(location, "code")
	if _, err = os.Stat(binaryPath); err != nil {
		return fmt.Errorf("find binary: %w", err)
	}

	return command.StartBackgroundOnce("vscode-web", func() (*exec.Cmd, error) {
		o.log.Infof("Starting vscode-web in background...")
		runCommand := fmt.Sprintf(
			"%s serve-web --accept-server-license-terms --without-connection-token "+
				"--host '%s' --port '%s' --server-data-dir '%s'",
			binaryPath, o.host, o.port, location,
		)
		args := []string{}
		if o.userName != "" {
			args = append(args, "su", o.userName, "-c", runCommand)
		} else {
			args = append(args, "sh", "-c", runCommand)
		}
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = location
		return cmd, nil
	})
}

func prepareVSCodeWebLocation(userName string) (string, error) {
	var err error
	homeFolder := ""
	if userName != "" {
		homeFolder, err = command.GetHome(userName)
	} else {
		homeFolder, err = util.UserHomeDir()
	}
	if err != nil {
		return "", err
	}

	folder := filepath.Join(homeFolder, ".vscode-web")
	// #nosec G301
	if err = os.MkdirAll(folder, 0o755); err != nil {
		return "", err
	}

	return folder, nil
}
```

- [ ] **Step 5: Run tests + build to verify pass**

Run: `go test ./pkg/ide/vscodeweb/... && go build ./...`
Expected: PASS; build succeeds (the two new config constants are unused for now, which is allowed for package-level consts).

- [ ] **Step 6: Commit**

```bash
git add pkg/config/ide.go pkg/ide/vscodeweb/
git commit -m "feat(ide): add vscode-web (code serve-web) IDE package"
```

---

### Task 2: `code-server` Go package (Coder)

**Files:**
- Create: `pkg/ide/codeserver/codeserver.go`
- Test: `pkg/ide/codeserver/codeserver_test.go`

**Interfaces:**
- Produces: package `codeserver` with `Options ide.Options`, `DefaultVSCodePort = 10800`, `func NewCodeServerServer(extensions []string, settings string, userName string, host, port string, values map[string]config.OptionValue, log log.Logger) *CodeServerServer`, methods `Install() error`, `Start() error`, `InstallExtensions() error`, and option key consts `BindAddressOption`, `OpenOption`, `ForwardPortsOption`.

- [ ] **Step 1: Write the failing test**

Create `pkg/ide/codeserver/codeserver_test.go`:

```go
package codeserver

import (
	"strings"
	"testing"

	"github.com/skevetter/devpod/pkg/config"
)

func TestGetReleaseUrlDefaults(t *testing.T) {
	o := &CodeServerServer{values: nil}
	url := o.getReleaseUrl()
	if !strings.Contains(url, "github.com/coder/code-server/releases/download/v4.126.0/code-server-4.126.0-linux-") {
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/ide/codeserver/...`
Expected: FAIL — `CodeServerServer` undefined.

- [ ] **Step 3: Write the implementation**

Create `pkg/ide/codeserver/codeserver.go`:

```go
package codeserver

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/pkg/command"
	"github.com/skevetter/devpod/pkg/config"
	copy2 "github.com/skevetter/devpod/pkg/copy"
	"github.com/skevetter/devpod/pkg/extract"
	devpodhttp "github.com/skevetter/devpod/pkg/http"
	"github.com/skevetter/devpod/pkg/ide"
	"github.com/skevetter/devpod/pkg/ide/vscode"
	"github.com/skevetter/devpod/pkg/util"
	"github.com/skevetter/log"
)

const (
	// code-server is Coder's standalone VS Code in the browser. The tarball is
	// self-contained (no runtime download). %s is the version, e.g. 4.126.0.
	DownloadAmd64Template = "https://github.com/coder/code-server/releases/download/v%s/code-server-%s-linux-amd64.tar.gz"
	DownloadArm64Template = "https://github.com/coder/code-server/releases/download/v%s/code-server-%s-linux-arm64.tar.gz"
)

const (
	ForwardPortsOption  = "FORWARD_PORTS"
	OpenOption          = "OPEN"
	BindAddressOption   = "BIND_ADDRESS"
	VersionOption       = "VERSION"
	DownloadAmd64Option = "DOWNLOAD_AMD64"
	DownloadArm64Option = "DOWNLOAD_ARM64"
)

var Options = ide.Options{
	ForwardPortsOption: {
		Name:        ForwardPortsOption,
		Description: "If DevPod should automatically do port-forwarding",
		Default:     "true",
		Enum:        []string{"true", "false"},
	},
	BindAddressOption: {
		Name:        BindAddressOption,
		Description: "The address to bind code-server to locally, e.g. 0.0.0.0:12345",
		Default:     "",
	},
	VersionOption: {
		Name:        VersionOption,
		Description: "The version for the code-server binary",
		Default:     "4.126.0",
	},
	OpenOption: {
		Name:        OpenOption,
		Description: "If DevPod should automatically open the browser",
		Default:     "true",
		Enum:        []string{"true", "false"},
	},
	DownloadArm64Option: {
		Name:        DownloadArm64Option,
		Description: "The download url for the arm64 code-server binary",
	},
	DownloadAmd64Option: {
		Name:        DownloadAmd64Option,
		Description: "The download url for the amd64 code-server binary",
	},
}

const DefaultVSCodePort = 10800

func NewCodeServerServer(
	extensions []string,
	settings string,
	userName string,
	host, port string,
	values map[string]config.OptionValue,
	log log.Logger,
) *CodeServerServer {
	return &CodeServerServer{
		values:     values,
		extensions: extensions,
		settings:   settings,
		userName:   userName,
		host:       host,
		port:       port,
		log:        log,
	}
}

type CodeServerServer struct {
	values     map[string]config.OptionValue
	extensions []string
	settings   string
	userName   string
	host       string
	port       string
	log        log.Logger
}

func (o *CodeServerServer) InstallExtensions() error {
	if err := o.installExtensions(); err != nil {
		return fmt.Errorf("install extensions: %w", err)
	}
	return nil
}

func (o *CodeServerServer) Install() error {
	location, err := prepareCodeServerLocation(o.userName)
	if err != nil {
		return err
	}

	// already installed?
	if _, err = os.Stat(filepath.Join(location, "bin", "code-server")); err == nil {
		return nil
	}

	url := o.getReleaseUrl()

	vscode.InstallAPKRequirements(o.log)

	resp, err := devpodhttp.GetHTTPClient().Get(url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	// Tarball top dir is code-server-<ver>-linux-<arch>/; StripLevels(1) yields bin/code-server.
	if err = extract.Extract(resp.Body, location, extract.StripLevels(1)); err != nil {
		return fmt.Errorf("extract code-server: %w", err)
	}

	if o.userName != "" {
		if err = copy2.ChownR(location, o.userName); err != nil {
			return fmt.Errorf("chown: %w", err)
		}
	}

	if err = o.installSettings(); err != nil {
		return fmt.Errorf("install settings: %w", err)
	}

	return nil
}

func (o *CodeServerServer) getReleaseUrl() string {
	var url string
	version := Options.GetValue(o.values, VersionOption)

	if runtime.GOARCH == "arm64" {
		url = Options.GetValue(o.values, DownloadArm64Option)
		if url == "" {
			url = fmt.Sprintf(DownloadArm64Template, version, version)
		}
	} else {
		url = Options.GetValue(o.values, DownloadAmd64Option)
		if url == "" {
			url = fmt.Sprintf(DownloadAmd64Template, version, version)
		}
	}

	return url
}

func (o *CodeServerServer) installExtensions() error {
	if len(o.extensions) == 0 {
		return nil
	}

	location, err := prepareCodeServerLocation(o.userName)
	if err != nil {
		return err
	}

	out := o.log.Writer(logrus.InfoLevel, false)
	defer func() { _ = out.Close() }()

	binaryPath := filepath.Join(location, "bin", "code-server")
	extensionsDir := filepath.Join(location, "extensions")
	for _, extension := range o.extensions {
		o.log.Info("Install extension " + extension + "...")
		runCommand := fmt.Sprintf(
			"%s --install-extension '%s' --extensions-dir '%s'",
			binaryPath, extension, extensionsDir,
		)
		args := []string{}
		if o.userName != "" {
			args = append(args, "su", o.userName, "-c", runCommand)
		} else {
			args = append(args, "sh", "-c", runCommand)
		}
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdout = out
		cmd.Stderr = out
		if err = cmd.Run(); err != nil {
			o.log.Errorf("failed installing extension: extension=%s, error=%v", extension, err)
		} else {
			o.log.Infof("installed extension: extension=%s", extension)
		}
	}

	return nil
}

func (o *CodeServerServer) installSettings() error {
	if len(o.settings) == 0 {
		return nil
	}

	location, err := prepareCodeServerLocation(o.userName)
	if err != nil {
		return err
	}

	// code-server reads user settings from <user-data-dir>/User/settings.json.
	settingsDir := filepath.Join(location, "data", "User")
	// #nosec G301
	if err = os.MkdirAll(settingsDir, 0o755); err != nil {
		return err
	}

	if err = os.WriteFile(filepath.Join(settingsDir, "settings.json"), []byte(o.settings), 0o600); err != nil {
		return err
	}

	return copy2.ChownR(location, o.userName)
}

func (o *CodeServerServer) Start() error {
	location, err := prepareCodeServerLocation(o.userName)
	if err != nil {
		return err
	}

	if o.host == "" {
		o.host = "0.0.0.0"
	}
	if o.port == "" {
		o.port = strconv.Itoa(DefaultVSCodePort)
	}

	binaryPath := filepath.Join(location, "bin", "code-server")
	if _, err = os.Stat(binaryPath); err != nil {
		return fmt.Errorf("find binary: %w", err)
	}

	dataDir := filepath.Join(location, "data")
	extensionsDir := filepath.Join(location, "extensions")
	return command.StartBackgroundOnce("code-server", func() (*exec.Cmd, error) {
		o.log.Infof("Starting code-server in background...")
		runCommand := fmt.Sprintf(
			"%s --bind-addr '%s:%s' --auth none --user-data-dir '%s' --extensions-dir '%s'",
			binaryPath, o.host, o.port, dataDir, extensionsDir,
		)
		args := []string{}
		if o.userName != "" {
			args = append(args, "su", o.userName, "-c", runCommand)
		} else {
			args = append(args, "sh", "-c", runCommand)
		}
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = location
		return cmd, nil
	})
}

func prepareCodeServerLocation(userName string) (string, error) {
	var err error
	homeFolder := ""
	if userName != "" {
		homeFolder, err = command.GetHome(userName)
	} else {
		homeFolder, err = util.UserHomeDir()
	}
	if err != nil {
		return "", err
	}

	folder := filepath.Join(homeFolder, ".code-server")
	// #nosec G301
	if err = os.MkdirAll(folder, 0o755); err != nil {
		return "", err
	}

	return folder, nil
}
```

- [ ] **Step 4: Run tests + build to verify pass**

Run: `go test ./pkg/ide/codeserver/... && go build ./...`
Expected: PASS; build succeeds.

- [ ] **Step 5: Commit**

```bash
git add pkg/ide/codeserver/
git commit -m "feat(ide): add code-server (Coder) IDE package"
```

---

### Task 3: Register both IDEs in the IDE registry

**Files:**
- Modify: `pkg/ide/ideparse/parse.go` (imports + two `AllowedIDE` entries)
- Test: `pkg/ide/ideparse/parse_registry_test.go`

**Interfaces:**
- Consumes: `vscodeweb.Options`, `codeserver.Options` (Task 1/2); `config.IDEVSCodeWeb`, `config.IDECodeServer`.
- Produces: `ideparse.GetIDEOptions("vscode-web")` and `("code-server")` return non-nil options.

- [ ] **Step 1: Write the failing test**

Create `pkg/ide/ideparse/parse_registry_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/ide/ideparse/... -run TestNewBrowserIDEsRegistered`
Expected: FAIL — `unrecognized ide 'vscode-web'`.

- [ ] **Step 3: Add imports**

In `pkg/ide/ideparse/parse.go`, add to the import block (keep alphabetical with the other `pkg/ide/*` imports):

```go
	"github.com/skevetter/devpod/pkg/ide/codeserver"
	"github.com/skevetter/devpod/pkg/ide/vscodeweb"
```

- [ ] **Step 4: Add the two registry entries**

In `pkg/ide/ideparse/parse.go`, insert directly after the `IDEOpenVSCode` entry (the block ending with `Group: config.IDEGroupPrimary,` for "VS Code Browser"):

```go
	{
		Name:         config.IDEVSCodeWeb,
		DisplayName:  "VS Code Web",
		Options:      vscodeweb.Options,
		Icon:         config.WebsiteAssetsURL + "/vscodebrowser.svg",
		Experimental: true,
		Group:        config.IDEGroupPrimary,
	},
	{
		Name:         config.IDECodeServer,
		DisplayName:  "code-server",
		Options:      codeserver.Options,
		Icon:         config.WebsiteAssetsURL + "/vscodebrowser.svg",
		Experimental: true,
		Group:        config.IDEGroupPrimary,
	},
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./pkg/ide/ideparse/... && go build ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/ide/ideparse/parse.go pkg/ide/ideparse/parse_registry_test.go
git commit -m "feat(ide): register vscode-web and code-server in IDE registry"
```

---

### Task 4: Agent container install wiring + async extension commands

**Files:**
- Modify: `cmd/agent/container/setup.go` (imports, `installIDE` cases, two `setupX` methods)
- Create: `cmd/agent/container/vscodeweb_async.go`
- Create: `cmd/agent/container/codeserver_async.go`
- Modify: `cmd/agent/container/container.go` (register async commands)

**Interfaces:**
- Consumes: `vscodeweb.NewVSCodeWebServer`, `vscodeweb.DefaultVSCodePort`, `codeserver.NewCodeServerServer`, `codeserver.DefaultVSCodePort`.
- Produces: `installIDE` handles `vscode-web`/`code-server`; cobra subcommands `vscodeweb-async` and `codeserver-async`.

- [ ] **Step 1: Add imports to setup.go**

In `cmd/agent/container/setup.go`, add to the import block alongside the other `pkg/ide/*` imports:

```go
	"github.com/skevetter/devpod/pkg/ide/codeserver"
	"github.com/skevetter/devpod/pkg/ide/vscodeweb"
```

- [ ] **Step 2: Add the dispatch cases**

In `installIDE` (after the `IDEOpenVSCode` case at line ~480), add:

```go
	case string(config2.IDEVSCodeWeb):
		return cmd.setupVSCodeWeb(setupInfo, ide.Options, log)
	case string(config2.IDECodeServer):
		return cmd.setupCodeServer(setupInfo, ide.Options, log)
```

- [ ] **Step 3: Add the two setup methods**

In `cmd/agent/container/setup.go`, directly after `setupOpenVSCode` (ends ~line 655), add:

```go
func (cmd *SetupContainerCmd) setupVSCodeWeb(
	setupInfo *config.Result,
	ideOptions map[string]config2.OptionValue,
	log log.Logger,
) error {
	log.Debugf("setup vscode-web")
	vsCodeConfiguration := config.GetVSCodeConfiguration(setupInfo.MergedConfig)
	settings := ""
	if len(vsCodeConfiguration.Settings) > 0 {
		out, err := json.Marshal(vsCodeConfiguration.Settings)
		if err != nil {
			return err
		}
		settings = string(out)
	}

	user := config.GetRemoteUser(setupInfo)
	server := vscodeweb.NewVSCodeWebServer(
		vsCodeConfiguration.Extensions,
		settings,
		user,
		"0.0.0.0",
		strconv.Itoa(vscodeweb.DefaultVSCodePort),
		ideOptions,
		log,
	)

	if err := server.Install(); err != nil {
		return err
	}

	if len(vsCodeConfiguration.Extensions) > 0 {
		err := command.StartBackgroundOnce("vscode-web-async", func() (*exec.Cmd, error) {
			log.Infof(
				"installing extensions in the background: %s",
				strings.Join(vsCodeConfiguration.Extensions, ","),
			)
			binaryPath, err := os.Executable()
			if err != nil {
				return nil, err
			}
			return exec.Command(
				binaryPath, "agent", "container", "vscodeweb-async",
				"--setup-info", cmd.SetupInfo,
			), nil
		})
		if err != nil {
			return fmt.Errorf("install extensions: %w", err)
		}
	}

	return server.Start()
}

func (cmd *SetupContainerCmd) setupCodeServer(
	setupInfo *config.Result,
	ideOptions map[string]config2.OptionValue,
	log log.Logger,
) error {
	log.Debugf("setup code-server")
	vsCodeConfiguration := config.GetVSCodeConfiguration(setupInfo.MergedConfig)
	settings := ""
	if len(vsCodeConfiguration.Settings) > 0 {
		out, err := json.Marshal(vsCodeConfiguration.Settings)
		if err != nil {
			return err
		}
		settings = string(out)
	}

	user := config.GetRemoteUser(setupInfo)
	server := codeserver.NewCodeServerServer(
		vsCodeConfiguration.Extensions,
		settings,
		user,
		"0.0.0.0",
		strconv.Itoa(codeserver.DefaultVSCodePort),
		ideOptions,
		log,
	)

	if err := server.Install(); err != nil {
		return err
	}

	if len(vsCodeConfiguration.Extensions) > 0 {
		err := command.StartBackgroundOnce("code-server-async", func() (*exec.Cmd, error) {
			log.Infof(
				"installing extensions in the background: %s",
				strings.Join(vsCodeConfiguration.Extensions, ","),
			)
			binaryPath, err := os.Executable()
			if err != nil {
				return nil, err
			}
			return exec.Command(
				binaryPath, "agent", "container", "codeserver-async",
				"--setup-info", cmd.SetupInfo,
			), nil
		})
		if err != nil {
			return fmt.Errorf("install extensions: %w", err)
		}
	}

	return server.Start()
}
```

- [ ] **Step 4: Create the vscode-web async command**

Create `cmd/agent/container/vscodeweb_async.go`:

```go
package container

import (
	"encoding/json"

	"github.com/skevetter/devpod/cmd/flags"
	"github.com/skevetter/devpod/pkg/compress"
	"github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/skevetter/devpod/pkg/ide/vscodeweb"
	"github.com/skevetter/log"
	"github.com/spf13/cobra"
)

// VSCodeWebAsyncCmd holds the cmd flags.
type VSCodeWebAsyncCmd struct {
	*flags.GlobalFlags

	SetupInfo string
}

// NewVSCodeWebAsyncCmd creates a new command.
func NewVSCodeWebAsyncCmd() *cobra.Command {
	cmd := &VSCodeWebAsyncCmd{}
	asyncCmd := &cobra.Command{
		Use:   "vscodeweb-async",
		Short: "Installs vscode-web extensions",
		Args:  cobra.NoArgs,
		RunE:  cmd.Run,
	}
	asyncCmd.Flags().StringVar(&cmd.SetupInfo, "setup-info", "", "The container setup info")
	_ = asyncCmd.MarkFlagRequired("setup-info")
	return asyncCmd
}

// Run runs the command logic.
func (cmd *VSCodeWebAsyncCmd) Run(_ *cobra.Command, _ []string) error {
	log.Default.Debugf("Start setting up container...")
	decompressed, err := compress.Decompress(cmd.SetupInfo)
	if err != nil {
		return err
	}

	setupInfo := &config.Result{}
	err = json.Unmarshal([]byte(decompressed), setupInfo)
	if err != nil {
		return err
	}

	vsCodeConfiguration := config.GetVSCodeConfiguration(setupInfo.MergedConfig)
	user := config.GetRemoteUser(setupInfo)
	return vscodeweb.NewVSCodeWebServer(vsCodeConfiguration.Extensions, "", user, "", "", nil, log.Default).
		InstallExtensions()
}
```

- [ ] **Step 5: Create the code-server async command**

Create `cmd/agent/container/codeserver_async.go`:

```go
package container

import (
	"encoding/json"

	"github.com/skevetter/devpod/cmd/flags"
	"github.com/skevetter/devpod/pkg/compress"
	"github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/skevetter/devpod/pkg/ide/codeserver"
	"github.com/skevetter/log"
	"github.com/spf13/cobra"
)

// CodeServerAsyncCmd holds the cmd flags.
type CodeServerAsyncCmd struct {
	*flags.GlobalFlags

	SetupInfo string
}

// NewCodeServerAsyncCmd creates a new command.
func NewCodeServerAsyncCmd() *cobra.Command {
	cmd := &CodeServerAsyncCmd{}
	asyncCmd := &cobra.Command{
		Use:   "codeserver-async",
		Short: "Installs code-server extensions",
		Args:  cobra.NoArgs,
		RunE:  cmd.Run,
	}
	asyncCmd.Flags().StringVar(&cmd.SetupInfo, "setup-info", "", "The container setup info")
	_ = asyncCmd.MarkFlagRequired("setup-info")
	return asyncCmd
}

// Run runs the command logic.
func (cmd *CodeServerAsyncCmd) Run(_ *cobra.Command, _ []string) error {
	log.Default.Debugf("Start setting up container...")
	decompressed, err := compress.Decompress(cmd.SetupInfo)
	if err != nil {
		return err
	}

	setupInfo := &config.Result{}
	err = json.Unmarshal([]byte(decompressed), setupInfo)
	if err != nil {
		return err
	}

	vsCodeConfiguration := config.GetVSCodeConfiguration(setupInfo.MergedConfig)
	user := config.GetRemoteUser(setupInfo)
	return codeserver.NewCodeServerServer(vsCodeConfiguration.Extensions, "", user, "", "", nil, log.Default).
		InstallExtensions()
}
```

- [ ] **Step 6: Register both async commands**

In `cmd/agent/container/container.go`, after `containerCmd.AddCommand(NewOpenVSCodeAsyncCmd())`:

```go
	containerCmd.AddCommand(NewVSCodeWebAsyncCmd())
	containerCmd.AddCommand(NewCodeServerAsyncCmd())
```

- [ ] **Step 7: Build + vet**

Run: `go build ./... && go vet ./cmd/agent/container/...`
Expected: succeeds, no errors.

- [ ] **Step 8: Commit**

```bash
git add cmd/agent/container/
git commit -m "feat(ide): wire vscode-web and code-server into agent container setup"
```

---

### Task 5: Browser opener wiring + SSH auth-sock reuse

**Files:**
- Modify: `pkg/ide/opener/opener.go` (imports, `browserIDEOpener` cases, two opener funcs)
- Modify: `pkg/ide/types.go` (`ReusesAuthSock`)

**Interfaces:**
- Consumes: `vscodeweb.Options`/`vscodeweb.DefaultVSCodePort`, `codeserver.Options`/`codeserver.DefaultVSCodePort`, existing `ParseAddressAndPort`, `tunnel.StartBrowserTunnel`, `makeDaemonStartFunc`, `open2.Open`, `gpg.ForwardAgent`.

- [ ] **Step 1: Add imports to opener.go**

In `pkg/ide/opener/opener.go`, add alongside the other `pkg/ide/*` imports:

```go
	"github.com/skevetter/devpod/pkg/ide/codeserver"
	"github.com/skevetter/devpod/pkg/ide/vscodeweb"
```

- [ ] **Step 2: Add dispatch cases**

In `browserIDEOpener` (after the `IDEOpenVSCode` case at line ~61):

```go
	case string(config.IDEVSCodeWeb):
		return openVSCodeWebBrowser, true
	case string(config.IDECodeServer):
		return openCodeServerBrowser, true
```

- [ ] **Step 3: Add the two opener functions**

In `pkg/ide/opener/opener.go`, directly after the existing `openVSCodeBrowser` function, add:

```go
func openVSCodeWebBrowser(
	ctx context.Context,
	ideOptions map[string]config.OptionValue,
	params Params,
) error {
	if params.GPGAgentForwarding {
		if err := gpg.ForwardAgent(params.Client, params.Log); err != nil {
			return err
		}
	}

	folder := params.Result.SubstitutionContext.ContainerWorkspaceFolder
	addr, vscodePort, err := ParseAddressAndPort(
		vscodeweb.Options.GetValue(ideOptions, vscodeweb.BindAddressOption),
		vscodeweb.DefaultVSCodePort,
	)
	if err != nil {
		return err
	}

	targetURL := fmt.Sprintf("http://localhost:%d/?folder=%s", vscodePort, folder)
	if vscodeweb.Options.GetValue(ideOptions, vscodeweb.OpenOption) == config.BoolTrue {
		go func() {
			if openErr := open2.Open(ctx, targetURL, params.Log); openErr != nil {
				params.Log.Errorf("error opening vscode-web: %v", openErr)
			}
			params.Log.Infof(
				"started vscode-web in browser mode. " +
					"Please keep this terminal open as long as you use the VS Code Web version",
			)
		}()
	}

	forwardPorts := vscodeweb.Options.GetValue(ideOptions, vscodeweb.ForwardPortsOption) == config.BoolTrue
	extraPorts := []string{fmt.Sprintf("%s:%d", addr, vscodeweb.DefaultVSCodePort)}
	return tunnel.StartBrowserTunnel(tunnel.BrowserTunnelParams{
		Ctx:              ctx,
		DevPodConfig:     params.DevPodConfig,
		Client:           params.Client,
		User:             params.User,
		TargetURL:        targetURL,
		ForwardPorts:     forwardPorts,
		ExtraPorts:       extraPorts,
		AuthSockID:       params.SSHAuthSockID,
		GitSSHSigningKey: params.GitSSHSigningKey,
		Logger:           params.Log,
		DaemonStartFunc:  makeDaemonStartFunc(params, forwardPorts, extraPorts),
	})
}

func openCodeServerBrowser(
	ctx context.Context,
	ideOptions map[string]config.OptionValue,
	params Params,
) error {
	if params.GPGAgentForwarding {
		if err := gpg.ForwardAgent(params.Client, params.Log); err != nil {
			return err
		}
	}

	folder := params.Result.SubstitutionContext.ContainerWorkspaceFolder
	addr, vscodePort, err := ParseAddressAndPort(
		codeserver.Options.GetValue(ideOptions, codeserver.BindAddressOption),
		codeserver.DefaultVSCodePort,
	)
	if err != nil {
		return err
	}

	targetURL := fmt.Sprintf("http://localhost:%d/?folder=%s", vscodePort, folder)
	if codeserver.Options.GetValue(ideOptions, codeserver.OpenOption) == config.BoolTrue {
		go func() {
			if openErr := open2.Open(ctx, targetURL, params.Log); openErr != nil {
				params.Log.Errorf("error opening code-server: %v", openErr)
			}
			params.Log.Infof(
				"started code-server in browser mode. " +
					"Please keep this terminal open as long as you use code-server",
			)
		}()
	}

	forwardPorts := codeserver.Options.GetValue(ideOptions, codeserver.ForwardPortsOption) == config.BoolTrue
	extraPorts := []string{fmt.Sprintf("%s:%d", addr, codeserver.DefaultVSCodePort)}
	return tunnel.StartBrowserTunnel(tunnel.BrowserTunnelParams{
		Ctx:              ctx,
		DevPodConfig:     params.DevPodConfig,
		Client:           params.Client,
		User:             params.User,
		TargetURL:        targetURL,
		ForwardPorts:     forwardPorts,
		ExtraPorts:       extraPorts,
		AuthSockID:       params.SSHAuthSockID,
		GitSSHSigningKey: params.GitSSHSigningKey,
		Logger:           params.Log,
		DaemonStartFunc:  makeDaemonStartFunc(params, forwardPorts, extraPorts),
	})
}
```

- [ ] **Step 4: Update ReusesAuthSock**

In `pkg/ide/types.go`, in `ReusesAuthSock`, add after the `IDEOpenVSCode` case:

```go
	case string(config.IDEVSCodeWeb):
		return true
	case string(config.IDECodeServer):
		return true
```

- [ ] **Step 5: Build + test the opener package**

Run: `go build ./... && go test ./pkg/ide/opener/... ./pkg/ide/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/ide/opener/opener.go pkg/ide/types.go
git commit -m "feat(ide): open vscode-web and code-server via browser tunnel"
```

---

### Task 6: Desktop app integration (experimental, default visible)

**Files:**
- Modify: `desktop/src/gen/Settings.ts`
- Modify: `desktop/src/contexts/SettingsContext/SettingsContext.tsx`
- Modify: `desktop/src/views/Settings/Settings.tsx`
- Modify: `desktop/src/useIDEs.ts`
- Modify: `desktop/src/types.ts`
- Modify: `desktop/src/components/IDEIcon/IDEIcon.tsx`

**Interfaces:**
- Consumes: backend IDE names `vscode-web`, `code-server` (surfaced via `devpod ide list`); existing `VSCodeBrowser` image import.
- Produces: settings keys `experimental_vscodeWeb`, `experimental_codeServer`; both IDEs visible in the picker by default.

- [ ] **Step 1: Add settings type fields**

In `desktop/src/gen/Settings.ts`, add after `experimental_bob: boolean`:

```ts
  experimental_bob: boolean
  experimental_vscodeWeb: boolean
  experimental_codeServer: boolean
  experimental_devPodPro: boolean
```

(Hand-edit only — do NOT regenerate via ts-rs; see Global Constraints.)

- [ ] **Step 2: Add settings defaults (visible)**

In `desktop/src/contexts/SettingsContext/SettingsContext.tsx`, add after `experimental_bob: true,`:

```ts
  experimental_bob: true,
  experimental_vscodeWeb: true,
  experimental_codeServer: true,
  experimental_devPodPro: false,
```

- [ ] **Step 3: Add the experimental toggles**

In `desktop/src/views/Settings/Settings.tsx`, in `ExperimentalSettings()`, after the `Bob` `HStack` block, add:

```tsx
        <HStack width="full" align="center">
          <Switch
            isChecked={settings.experimental_vscodeWeb}
            onChange={(e) => set("experimental_vscodeWeb", e.target.checked)}
          />
          <FormLabel marginBottom="0" whiteSpace="nowrap" fontSize="sm">
            VS Code Web
          </FormLabel>
        </HStack>

        <HStack width="full" align="center">
          <Switch
            isChecked={settings.experimental_codeServer}
            onChange={(e) => set("experimental_codeServer", e.target.checked)}
          />
          <FormLabel marginBottom="0" whiteSpace="nowrap" fontSize="sm">
            code-server
          </FormLabel>
        </HStack>
```

- [ ] **Step 4: Add the IDE filter entries**

In `desktop/src/useIDEs.ts`, add the name consts after `const BOB = "bob"`:

```ts
const VSCODE_WEB = "vscode-web"
const CODE_SERVER = "code-server"
```

and add the filter lines after the `BOB` line:

```ts
        if (ide.name === VSCODE_WEB && settings.experimental_vscodeWeb) return true
        if (ide.name === CODE_SERVER && settings.experimental_codeServer) return true
```

- [ ] **Step 5: Add to SUPPORTED_IDES**

In `desktop/src/types.ts`, add to the `SUPPORTED_IDES` array (after `"bob",`):

```ts
  "bob",
  "vscode-web",
  "code-server",
] as const
```

- [ ] **Step 6: Map the icons**

In `desktop/src/components/IDEIcon/IDEIcon.tsx`, add to the `IDE_ICONS` object (after `bob: BobSvg,`):

```ts
  bob: BobSvg,
  "vscode-web": VSCodeBrowser,
  "code-server": VSCodeBrowser,
}
```

(`VSCodeBrowser` is already imported in this file.)

- [ ] **Step 7: Type-check / build the desktop app**

Run: `cd desktop && npm install && npm run build`
Expected: build/type-check succeeds with no TS errors. (If the project exposes a lighter `npm run lint`/`tsc --noEmit`, that is sufficient for this task.)

- [ ] **Step 8: Commit**

```bash
git add desktop/src/
git commit -m "feat(desktop): add vscode-web and code-server IDEs to the app"
```

---

### Task 7: e2e coverage

**Files:**
- Modify: `e2e/tests/ide/ide.go`

**Interfaces:**
- Consumes: the fully wired IDEs from Tasks 1-6; existing `f.DevPodUpWithIDE` helper.

- [ ] **Step 1: Add the two e2e cases**

In `e2e/tests/ide/ide.go`, after the existing `openvscode` block (lines ~39-40), add:

```go
		err = f.DevPodUpWithIDE(ctx, tempDir, "--open-ide=false", "--ide=vscode-web")
		framework.ExpectNoError(err)

		err = f.DevPodUpWithIDE(ctx, tempDir, "--open-ide=false", "--ide=code-server")
		framework.ExpectNoError(err)
```

- [ ] **Step 2: Compile the e2e package**

Run: `go vet ./e2e/...`
Expected: succeeds (the suite needs a Docker provider to actually run — see Verification).

- [ ] **Step 3: Commit**

```bash
git add e2e/tests/ide/ide.go
git commit -m "test(e2e): cover vscode-web and code-server IDEs"
```

---

## Self-Review

**Spec coverage:** Two backends (Tasks 1-2) ✓; add-alongside, openvscode untouched ✓; registry (Task 3) ✓; agent install + async (Task 4) ✓; opener + auth-sock (Task 5) ✓; full desktop UI, experimental, default-on (Task 6) ✓; e2e + unit tests (Tasks 1,2,7) ✓; distinct install dirs, `--accept-server-license-terms`, no `go.mod`/`settings.rs` churn (Global Constraints) ✓.

**Type consistency:** `NewVSCodeWebServer`/`VSCodeWebServer` and `NewCodeServerServer`/`CodeServerServer` are referenced identically in their packages, the async commands, and `setup.go`. Option key consts (`BindAddressOption`, `OpenOption`, `ForwardPortsOption`) and `DefaultVSCodePort` are referenced from the opener exactly as defined. Config constants `IDEVSCodeWeb`/`IDECodeServer` are defined once (Task 1) and consumed in Tasks 3-5. Settings keys `experimental_vscodeWeb`/`experimental_codeServer` match across `gen/Settings.ts`, `SettingsContext.tsx`, `Settings.tsx`, and `useIDEs.ts`.

## Verification (end-to-end)

1. `go build ./...`, `go vet ./...`, `golangci-lint run` (if configured) — clean.
2. `go test ./pkg/ide/vscodeweb/... ./pkg/ide/codeserver/... ./pkg/ide/ideparse/...` — pass.
3. Build the agent/CLI, then `devpod ide list` lists `vscode-web` and `code-server`.
4. Manual (Docker provider), for each IDE: `devpod up <ws> --ide=vscode-web` and `--ide=code-server` — confirm the browser opens an up-to-date VS Code at `http://localhost:10800/?folder=...`, the workspace folder loads, and the process stays up. Add a `settings`/`extensions` entry to `.devcontainer.json` and confirm it is applied (parity check; if `serve-web` ignores the seeded settings dir, that is a non-blocking follow-up — the core acceptance criterion is met).
5. e2e: `--ide=vscode-web` and `--ide=code-server` cases pass under the docker-provider IDE suite.
6. Desktop: `cd desktop && npm run build` passes; both IDEs appear in the picker (visible by default) and their Experimental toggles work.
