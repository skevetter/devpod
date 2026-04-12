# cmd/up.go Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract the 1839-line `cmd/up.go` into focused `pkg/` packages with unit tests, leaving ~350-400 lines of command orchestration.

**Architecture:** Six commits, each extracting one responsibility area into a `pkg/` package (or extending an existing one), adding tests, and updating `cmd/up.go` to call the new package. Pure refactor -- no behavior changes.

**Tech Stack:** Go, cobra, testify/assert, table-driven tests

**Spec:** `docs/superpowers/specs/2026-04-11-cmd-up-refactor-design.md`

---

## File Structure

| Action | Path | Responsibility |
|--------|------|----------------|
| Create | `pkg/dotfiles/dotfiles.go` | Dotfiles setup orchestration |
| Create | `pkg/dotfiles/dotfiles_test.go` | Tests for pure helper functions |
| Create | `pkg/ide/opener/opener.go` | IDE launch dispatch and helpers |
| Create | `pkg/ide/opener/opener_test.go` | Tests for parseAddressAndPort, dispatch routing |
| Create | `pkg/gpg/forward.go` | GPG agent forwarding |
| Create | `pkg/gpg/forward_test.go` | Tests for command construction |
| Modify | `pkg/tunnel/browser.go` (create) | Browser tunnel + backhaul |
| Create | `pkg/tunnel/browser_test.go` | Tests for SSH command construction |
| Modify | `pkg/workspace/provider.go` | Add CheckProviderUpdate, GetProInstance |
| Create | `pkg/workspace/provider_update_test.go` | Tests for version comparison |
| Modify | `cmd/up.go` | Slim down to orchestration only |

---

### Task 1: Extract `pkg/dotfiles/`

**Files:**
- Create: `pkg/dotfiles/dotfiles.go`
- Create: `pkg/dotfiles/dotfiles_test.go`
- Modify: `cmd/up.go:1384-1553` (remove moved functions)

- [ ] **Step 1: Write failing tests for pure helper functions**

Create `pkg/dotfiles/dotfiles_test.go`:

```go
package dotfiles

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractKeysFromEnvKeyValuePairs(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "empty input",
			input: []string{},
			want:  []string{},
		},
		{
			name:  "single pair",
			input: []string{"FOO=bar"},
			want:  []string{"FOO"},
		},
		{
			name:  "multiple pairs",
			input: []string{"FOO=bar", "BAZ=qux"},
			want:  []string{"FOO", "BAZ"},
		},
		{
			name:  "value with equals sign",
			input: []string{"FOO=bar=baz"},
			want:  []string{"FOO"},
		},
		{
			name:  "no equals sign skipped",
			input: []string{"INVALID"},
			want:  []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractKeysFromEnvKeyValuePairs(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCollectDotfilesScriptEnvKeyValuePairs(t *testing.T) {
	t.Run("empty file list", func(t *testing.T) {
		got, err := collectDotfilesScriptEnvKeyValuePairs(nil)
		assert.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("reads key-value pairs from file", func(t *testing.T) {
		dir := t.TempDir()
		envFile := dir + "/test.env"
		err := os.WriteFile(envFile, []byte("FOO=bar\nBAZ=qux\n"), 0o600)
		assert.NoError(t, err)

		got, err := collectDotfilesScriptEnvKeyValuePairs([]string{envFile})
		assert.NoError(t, err)
		assert.Contains(t, got, "FOO=bar")
		assert.Contains(t, got, "BAZ=qux")
	})

	t.Run("nonexistent file returns error", func(t *testing.T) {
		_, err := collectDotfilesScriptEnvKeyValuePairs([]string{"/nonexistent/file"})
		assert.Error(t, err)
	})
}

func TestBuildDotCmdAgentArguments(t *testing.T) {
	tests := []struct {
		name     string
		repo     string
		script   string
		strict   bool
		debug    bool
		wantArgs []string
	}{
		{
			name: "basic repo only",
			repo: "https://github.com/user/dots",
			wantArgs: []string{
				"agent", "workspace", "install-dotfiles",
				"--repository", "https://github.com/user/dots",
			},
		},
		{
			name:   "with script",
			repo:   "https://github.com/user/dots",
			script: "install.sh",
			wantArgs: []string{
				"agent", "workspace", "install-dotfiles",
				"--repository", "https://github.com/user/dots",
				"--install-script", "install.sh",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildDotCmdAgentArguments(tt.repo, tt.script, tt.strict, tt.debug)
			assert.Equal(t, tt.wantArgs, got)
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd pkg/dotfiles && go test -v ./...`
Expected: FAIL -- package doesn't exist yet

- [ ] **Step 3: Create `pkg/dotfiles/dotfiles.go`**

Move the following functions from `cmd/up.go` lines 1384-1553 into `pkg/dotfiles/dotfiles.go`:
- `setupDotfiles` -> exported as `Setup`
- `buildDotCmd` -> unexported
- `buildDotCmdAgentArguments` -> unexported (refactored to accept booleans instead of config/logger)
- `extractKeysFromEnvKeyValuePairs` -> unexported
- `collectDotfilesScriptEnvKeyvaluePairs` -> unexported (renamed to `collectDotfilesScriptEnvKeyValuePairs`)

```go
package dotfiles

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/pkg/agent"
	client2 "github.com/skevetter/devpod/pkg/client"
	"github.com/skevetter/devpod/pkg/config"
	config2 "github.com/skevetter/devpod/pkg/devcontainer/config"
	devssh "github.com/skevetter/devpod/pkg/ssh"
	"github.com/skevetter/log"
)

// SetupParams holds all parameters for dotfiles setup.
type SetupParams struct {
	Source       string
	Script       string
	EnvFiles     []string
	EnvKeyValues []string
	Client       client2.BaseWorkspaceClient
	DevPodConfig *config.Config
	Log          log.Logger
}

// Setup clones and installs dotfiles into the devcontainer.
func Setup(params SetupParams) error {
	dotfilesRepo := params.DevPodConfig.ContextOption(config.ContextOptionDotfilesURL)
	if params.Source != "" {
		dotfilesRepo = params.Source
	}

	dotfilesScript := params.DevPodConfig.ContextOption(config.ContextOptionDotfilesScript)
	if params.Script != "" {
		dotfilesScript = params.Script
	}

	if dotfilesRepo == "" {
		params.Log.Debug("No dotfiles repo specified, skipping")
		return nil
	}

	params.Log.Infof("Dotfiles Git repository %s specified", dotfilesRepo)
	params.Log.Debug("Cloning dotfiles into the devcontainer...")

	strictHostKey := params.DevPodConfig.ContextOption(
		config.ContextOptionSSHStrictHostKeyChecking,
	) == config.BoolTrue

	dotCmd, err := buildDotCmd(
		dotfilesRepo,
		dotfilesScript,
		params.EnvFiles,
		params.EnvKeyValues,
		params.Client,
		strictHostKey,
		params.Log,
	)
	if err != nil {
		return err
	}
	if params.Log.GetLevel() == logrus.DebugLevel {
		dotCmd.Args = append(dotCmd.Args, "--debug")
	}

	params.Log.Debugf("Running dotfiles setup command: %v", dotCmd.Args)

	writer := params.Log.Writer(logrus.InfoLevel, false)

	dotCmd.Stdout = writer
	dotCmd.Stderr = writer

	err = dotCmd.Run()
	if err != nil {
		return err
	}

	params.Log.Infof("Done setting up dotfiles into the devcontainer")

	return nil
}

func buildDotCmdAgentArguments(
	dotfilesRepo, dotfilesScript string,
	strictHostKey, debug bool,
) []string {
	agentArguments := []string{
		"agent",
		"workspace",
		"install-dotfiles",
		"--repository",
		dotfilesRepo,
	}

	if strictHostKey {
		agentArguments = append(agentArguments, "--strict-host-key-checking")
	}

	if debug {
		agentArguments = append(agentArguments, "--debug")
	}

	if dotfilesScript != "" {
		agentArguments = append(agentArguments, "--install-script", dotfilesScript)
	}

	return agentArguments
}

func buildDotCmd(
	dotfilesRepo, dotfilesScript string,
	envFiles, envKeyValuePairs []string,
	client client2.BaseWorkspaceClient,
	strictHostKey bool,
	logger log.Logger,
) (*exec.Cmd, error) {
	sshCmd := []string{
		"ssh",
		"--agent-forwarding=true",
		"--start-services=true",
	}

	envFilesKeyValuePairs, err := collectDotfilesScriptEnvKeyValuePairs(envFiles)
	if err != nil {
		return nil, err
	}

	allEnvKeyValuesPairs := slices.Concat(envFilesKeyValuePairs, envKeyValuePairs)
	allEnvKeys := extractKeysFromEnvKeyValuePairs(allEnvKeyValuesPairs)
	for _, envKey := range allEnvKeys {
		sshCmd = append(sshCmd, "--send-env", envKey)
	}

	remoteUser, err := devssh.GetUser(
		client.WorkspaceConfig().ID,
		client.WorkspaceConfig().SSHConfigPath,
		client.WorkspaceConfig().SSHConfigIncludePath,
	)
	if err != nil {
		remoteUser = "root"
	}

	agentArguments := buildDotCmdAgentArguments(
		dotfilesRepo, dotfilesScript,
		strictHostKey,
		logger.GetLevel() == logrus.DebugLevel,
	)
	sshCmd = append(sshCmd,
		"--user",
		remoteUser,
		"--context",
		client.Context(),
		client.Workspace(),
		"--log-output=raw",
		"--command",
		agent.ContainerDevPodHelperLocation+" "+strings.Join(agentArguments, " "),
	)
	execPath, err := os.Executable()
	if err != nil {
		return nil, err
	}

	dotCmd := exec.Command(
		execPath,
		sshCmd...,
	)

	dotCmd.Env = append(dotCmd.Environ(), allEnvKeyValuesPairs...)
	return dotCmd, nil
}

func extractKeysFromEnvKeyValuePairs(envKeyValuePairs []string) []string {
	keys := []string{}
	for _, env := range envKeyValuePairs {
		keyValue := strings.SplitN(env, "=", 2)
		if len(keyValue) == 2 {
			keys = append(keys, keyValue[0])
		}
	}
	return keys
}

func collectDotfilesScriptEnvKeyValuePairs(envFiles []string) ([]string, error) {
	keyValues := []string{}
	for _, file := range envFiles {
		envFromFile, err := config2.ParseKeyValueFile(file)
		if err != nil {
			return nil, err
		}
		keyValues = append(keyValues, envFromFile...)
	}
	return keyValues, nil
}
```

- [ ] **Step 4: Add `import "os"` to test file and run tests**

Run: `cd pkg/dotfiles && go test -v ./...`
Expected: PASS for all helper function tests

- [ ] **Step 5: Update `cmd/up.go` to use `pkg/dotfiles`**

Replace the `setupDotfiles` call in `cmd/up.go` `configureWorkspace` method (around line 385):

```go
// Old:
if err := setupDotfiles(
    cmd.DotfilesSource,
    cmd.DotfilesScript,
    cmd.DotfilesScriptEnvFile,
    cmd.DotfilesScriptEnv,
    client,
    devPodConfig,
    log,
); err != nil {
    return err
}

// New:
if err := dotfiles.Setup(dotfiles.SetupParams{
    Source:       cmd.DotfilesSource,
    Script:       cmd.DotfilesScript,
    EnvFiles:     cmd.DotfilesScriptEnvFile,
    EnvKeyValues: cmd.DotfilesScriptEnv,
    Client:       client,
    DevPodConfig: devPodConfig,
    Log:          log,
}); err != nil {
    return err
}
```

Remove the following functions from `cmd/up.go`: `setupDotfiles`, `buildDotCmd`, `buildDotCmdAgentArguments`, `extractKeysFromEnvKeyValuePairs`, `collectDotfilesScriptEnvKeyvaluePairs`.

Add import: `"github.com/skevetter/devpod/pkg/dotfiles"`

Remove now-unused imports from `cmd/up.go`: `"slices"` (if no other uses remain).

- [ ] **Step 6: Verify build and existing tests pass**

Run: `go build ./... && go test ./cmd/... ./pkg/dotfiles/...`
Expected: BUILD OK, PASS

- [ ] **Step 7: Commit**

```bash
git add pkg/dotfiles/ cmd/up.go
git commit -m "$(cat <<'EOF'
refactor: extract pkg/dotfiles for dotfiles setup

Move dotfiles cloning and installation logic from cmd/up.go into a
dedicated pkg/dotfiles package. Add unit tests for pure helper functions.
EOF
)"
```

---

### Task 2: Extract `pkg/ide/opener/`

**Files:**
- Create: `pkg/ide/opener/opener.go`
- Create: `pkg/ide/opener/opener_test.go`
- Modify: `cmd/up.go:400-583, 840-1056` (remove moved functions)

- [ ] **Step 1: Write failing tests**

Create `pkg/ide/opener/opener_test.go`:

```go
package opener

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseAddressAndPort(t *testing.T) {
	tests := []struct {
		name        string
		bindAddr    string
		defaultPort int
		wantAddr    string
		wantPort    int
		wantErr     bool
	}{
		{
			name:        "empty uses default port",
			bindAddr:    "",
			defaultPort: 8080,
			wantAddr:    "",  // will be the found port as string
			wantPort:    0,   // will be >= defaultPort
			wantErr:     false,
		},
		{
			name:        "explicit host:port",
			bindAddr:    "127.0.0.1:9090",
			defaultPort: 8080,
			wantAddr:    "127.0.0.1:9090",
			wantPort:    9090,
			wantErr:     false,
		},
		{
			name:        "missing port returns error",
			bindAddr:    "127.0.0.1:",
			defaultPort: 8080,
			wantErr:     true,
		},
		{
			name:        "invalid format returns error",
			bindAddr:    "not-a-host-port",
			defaultPort: 8080,
			wantErr:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, port, err := ParseAddressAndPort(tt.bindAddr, tt.defaultPort)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			if tt.bindAddr == "" {
				// When empty, a free port is found
				assert.Greater(t, port, 0)
			} else {
				assert.Equal(t, tt.wantAddr, addr)
				assert.Equal(t, tt.wantPort, port)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd pkg/ide/opener && go test -v ./...`
Expected: FAIL -- package doesn't exist yet

- [ ] **Step 3: Create `pkg/ide/opener/opener.go`**

Move the following from `cmd/up.go`:
- `ideOpener` struct and methods -> restructured as package-level `Open` function
- `openVSCodeFlavor` -> unexported
- `openJetBrains` -> unexported
- `startVSCodeInBrowser` -> unexported
- `startJupyterNotebookInBrowser` -> unexported
- `startRStudioInBrowser` -> unexported
- `startFleet` -> unexported
- `parseAddressAndPort` -> exported as `ParseAddressAndPort` (used by tests, useful utility)

```go
package opener

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
	client2 "github.com/skevetter/devpod/pkg/client"
	"github.com/skevetter/devpod/pkg/command"
	"github.com/skevetter/devpod/pkg/config"
	config2 "github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/skevetter/devpod/pkg/ide/fleet"
	"github.com/skevetter/devpod/pkg/ide/jetbrains"
	"github.com/skevetter/devpod/pkg/ide/jupyter"
	"github.com/skevetter/devpod/pkg/ide/openvscode"
	"github.com/skevetter/devpod/pkg/ide/rstudio"
	"github.com/skevetter/devpod/pkg/ide/vscode"
	"github.com/skevetter/devpod/pkg/ide/zed"
	open2 "github.com/skevetter/devpod/pkg/open"
	"github.com/skevetter/devpod/pkg/port"
	"github.com/skevetter/log"
	"github.com/skratchdot/open-golang/open"
)

// Params holds everything the IDE opener needs from the command layer.
type Params struct {
	GPGAgentForwarding bool
	SSHAuthSockID      string
	GitSSHSigningKey   string
	DevPodConfig       *config.Config
	Client             client2.BaseWorkspaceClient
	User               string
	Result             *config2.Result
	Log                log.Logger
}

// Open launches the appropriate IDE based on ideName.
func Open(
	ctx context.Context,
	ideName string,
	ideOptions map[string]config.OptionValue,
	params Params,
) error {
	folder := params.Result.SubstitutionContext.ContainerWorkspaceFolder
	workspace := params.Client.Workspace()
	user := params.User

	switch ideName {
	case string(config.IDEVSCode), string(config.IDEVSCodeInsiders), string(config.IDECursor),
		string(config.IDECodium), string(config.IDEPositron), string(config.IDEWindsurf),
		string(config.IDEAntigravity), string(config.IDEBob):
		return openVSCodeFlavor(ctx, ideName, folder, workspace, ideOptions, params.Log)

	case string(config.IDERustRover), string(config.IDEGoland), string(config.IDEPyCharm),
		string(config.IDEPhpStorm), string(config.IDEIntellij), string(config.IDECLion),
		string(config.IDERider), string(config.IDERubyMine), string(config.IDEWebStorm),
		string(config.IDEDataSpell):
		return openJetBrains(ideName, folder, workspace, user, ideOptions, params.Log)

	case string(config.IDEOpenVSCode):
		return startVSCodeInBrowser(
			params.GPGAgentForwarding, ctx, params.DevPodConfig, params.Client,
			folder, user, ideOptions, params.SSHAuthSockID, params.GitSSHSigningKey, params.Log,
		)

	case string(config.IDEFleet):
		return startFleet(ctx, params.Client, params.Log)

	case string(config.IDEZed):
		return zed.Open(ctx, ideOptions, user, folder, workspace, params.Log)

	case string(config.IDEJupyterNotebook):
		return startJupyterNotebookInBrowser(
			params.GPGAgentForwarding, ctx, params.DevPodConfig, params.Client,
			user, ideOptions, params.SSHAuthSockID, params.GitSSHSigningKey, params.Log,
		)

	case string(config.IDERStudio):
		return startRStudioInBrowser(
			params.GPGAgentForwarding, ctx, params.DevPodConfig, params.Client,
			user, ideOptions, params.SSHAuthSockID, params.GitSSHSigningKey, params.Log,
		)

	default:
		return nil
	}
}

// ParseAddressAndPort parses a bind address option into address and port.
// If bindAddressOption is empty, finds an available port starting from defaultPort.
func ParseAddressAndPort(bindAddressOption string, defaultPort int) (string, int, error) {
	if bindAddressOption == "" {
		portName, err := port.FindAvailablePort(defaultPort)
		if err != nil {
			return "", 0, err
		}
		return fmt.Sprintf("%d", portName), portName, nil
	}

	address := bindAddressOption
	_, portStr, err := net.SplitHostPort(address)
	if err != nil {
		return "", 0, fmt.Errorf("parse host:port: %w", err)
	}
	if portStr == "" {
		return "", 0, fmt.Errorf("parse ADDRESS: expected host:port, got %s", address)
	}

	portName, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, fmt.Errorf("parse host:port: %w", err)
	}

	return address, portName, nil
}

func openVSCodeFlavor(
	ctx context.Context,
	ideName, folder, workspace string,
	ideOptions map[string]config.OptionValue,
	logger log.Logger,
) error {
	flavorMap := map[string]vscode.Flavor{
		string(config.IDEVSCode):        vscode.FlavorStable,
		string(config.IDEVSCodeInsiders): vscode.FlavorInsiders,
		string(config.IDECursor):        vscode.FlavorCursor,
		string(config.IDECodium):        vscode.FlavorCodium,
		string(config.IDEPositron):      vscode.FlavorPositron,
		string(config.IDEWindsurf):      vscode.FlavorWindsurf,
		string(config.IDEAntigravity):   vscode.FlavorAntigravity,
		string(config.IDEBob):           vscode.FlavorBob,
	}

	params := vscode.OpenParams{
		Workspace: workspace,
		Folder:    folder,
		NewWindow: vscode.Options.GetValue(ideOptions, vscode.OpenNewWindow) == config.BoolTrue,
		Flavor:    flavorMap[ideName],
		Log:       logger,
	}

	return vscode.Open(ctx, params)
}

func openJetBrains(
	ideName, folder, workspace, user string,
	ideOptions map[string]config.OptionValue,
	logger log.Logger,
) error {
	type jetbrainsFactory func() interface{ OpenGateway(string, string) error }

	jetbrainsMap := map[string]jetbrainsFactory{
		string(config.IDERustRover): func() interface{ OpenGateway(string, string) error } {
			return jetbrains.NewRustRoverServer(user, ideOptions, logger)
		},
		string(config.IDEGoland): func() interface{ OpenGateway(string, string) error } {
			return jetbrains.NewGolandServer(user, ideOptions, logger)
		},
		string(config.IDEPyCharm): func() interface{ OpenGateway(string, string) error } {
			return jetbrains.NewPyCharmServer(user, ideOptions, logger)
		},
		string(config.IDEPhpStorm): func() interface{ OpenGateway(string, string) error } {
			return jetbrains.NewPhpStorm(user, ideOptions, logger)
		},
		string(config.IDEIntellij): func() interface{ OpenGateway(string, string) error } {
			return jetbrains.NewIntellij(user, ideOptions, logger)
		},
		string(config.IDECLion): func() interface{ OpenGateway(string, string) error } {
			return jetbrains.NewCLionServer(user, ideOptions, logger)
		},
		string(config.IDERider): func() interface{ OpenGateway(string, string) error } {
			return jetbrains.NewRiderServer(user, ideOptions, logger)
		},
		string(config.IDERubyMine): func() interface{ OpenGateway(string, string) error } {
			return jetbrains.NewRubyMineServer(user, ideOptions, logger)
		},
		string(config.IDEWebStorm): func() interface{ OpenGateway(string, string) error } {
			return jetbrains.NewWebStormServer(user, ideOptions, logger)
		},
		string(config.IDEDataSpell): func() interface{ OpenGateway(string, string) error } {
			return jetbrains.NewDataSpellServer(user, ideOptions, logger)
		},
	}

	if factory, ok := jetbrainsMap[ideName]; ok {
		return factory().OpenGateway(folder, workspace)
	}
	return fmt.Errorf("unknown JetBrains IDE: %s", ideName)
}

// startVSCodeInBrowser, startJupyterNotebookInBrowser, startRStudioInBrowser, startFleet
// are moved here verbatim from cmd/up.go. They call into pkg/tunnel for browser tunnels
// and pkg/gpg for GPG forwarding (after those packages are extracted in Tasks 3-4).
// Until then, they import the functions directly. The full code for these functions
// is identical to cmd/up.go lines 840-993 and 995-1056.
```

**Note for implementer:** The `startVSCodeInBrowser`, `startJupyterNotebookInBrowser`, `startRStudioInBrowser`, and `startFleet` functions should be copied verbatim from `cmd/up.go` into `pkg/ide/opener/opener.go`. This temporarily duplicates `performGpgForwarding`, `startBrowserTunnel`, and `createSSHCommand` — they exist in both `cmd/up.go` (still used by other callers) and `pkg/ide/opener/`. Tasks 3-4 resolve this: `performGpgForwarding` moves to `pkg/gpg`, `startBrowserTunnel` and `createSSHCommand` move to `pkg/tunnel`, and `pkg/ide/opener` is updated to import from those packages. The duplication lives for exactly 2 commits.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd pkg/ide/opener && go test -v ./...`
Expected: PASS

- [ ] **Step 5: Update `cmd/up.go` to use `pkg/ide/opener`**

Replace the `openIDE` method in `cmd/up.go`:

```go
// Old:
func (cmd *UpCmd) openIDE(
	ctx context.Context,
	devPodConfig *config.Config,
	client client2.BaseWorkspaceClient,
	wctx *workspaceContext,
	log log.Logger,
) error {
	if !cmd.OpenIDE {
		return nil
	}
	ideConfig := client.WorkspaceConfig().IDE
	o := newIDEOpener(cmd, devPodConfig, client, wctx, log)
	return o.open(ctx, ideConfig.Name, ideConfig.Options)
}

// New:
func (cmd *UpCmd) openIDE(
	ctx context.Context,
	devPodConfig *config.Config,
	client client2.BaseWorkspaceClient,
	wctx *workspaceContext,
	log log.Logger,
) error {
	if !cmd.OpenIDE {
		return nil
	}
	ideConfig := client.WorkspaceConfig().IDE
	return opener.Open(ctx, ideConfig.Name, ideConfig.Options, opener.Params{
		GPGAgentForwarding: cmd.GPGAgentForwarding,
		SSHAuthSockID:      cmd.SSHAuthSockID,
		GitSSHSigningKey:   cmd.GitSSHSigningKey,
		DevPodConfig:       devPodConfig,
		Client:             client,
		User:               wctx.user,
		Result:             wctx.result,
		Log:                log,
	})
}
```

Remove from `cmd/up.go`: `ideOpener` struct, `newIDEOpener`, `ideOpener.open`, `ideOpener.openVSCodeFlavor`, `ideOpener.openJetBrains`, `startVSCodeInBrowser`, `startJupyterNotebookInBrowser`, `startRStudioInBrowser`, `startFleet`, `parseAddressAndPort`.

Add import: `"github.com/skevetter/devpod/pkg/ide/opener"`

Remove now-unused imports: IDE-specific packages (`vscode`, `jetbrains`, `jupyter`, `openvscode`, `rstudio`, `fleet`, `zed`), `"net"`, `open2`, `"github.com/skratchdot/open-golang/open"`.

- [ ] **Step 6: Verify build and tests pass**

Run: `go build ./... && go test ./cmd/... ./pkg/ide/opener/...`
Expected: BUILD OK, PASS

- [ ] **Step 7: Commit**

```bash
git add pkg/ide/opener/ cmd/up.go
git commit -m "$(cat <<'EOF'
refactor: extract pkg/ide/opener for IDE launch dispatch

Move IDE opening logic (VSCode, JetBrains, browser-based IDEs, Fleet, Zed)
from cmd/up.go into a dedicated pkg/ide/opener package with unit tests.
EOF
)"
```

---

### Task 3: Extract `pkg/gpg/`

**Files:**
- Create: `pkg/gpg/forward.go`
- Create: `pkg/gpg/forward_test.go`
- Modify: `pkg/ide/opener/opener.go` (update imports to use `pkg/gpg`)

- [ ] **Step 1: Write failing test**

Create `pkg/gpg/forward_test.go`:

```go
package gpg

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildForwardCommand(t *testing.T) {
	args := buildForwardArgs("/usr/bin/devpod", "root", "test-context", "test-workspace")
	assert.Contains(t, args, "ssh")
	assert.Contains(t, args, "--gpg-agent-forwarding=true")
	assert.Contains(t, args, "--agent-forwarding=true")
	assert.Contains(t, args, "--user")
	assert.Contains(t, args, "root")
	assert.Contains(t, args, "--context")
	assert.Contains(t, args, "test-context")
	assert.Contains(t, args, "test-workspace")
	assert.Contains(t, args, "--command")
	assert.Contains(t, args, "sleep infinity")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd pkg/gpg && go test -v ./...`
Expected: FAIL

- [ ] **Step 3: Create `pkg/gpg/forward.go`**

```go
package gpg

import (
	"os"
	"os/exec"

	client2 "github.com/skevetter/devpod/pkg/client"
	devssh "github.com/skevetter/devpod/pkg/ssh"
	"github.com/skevetter/log"
)

// ForwardAgent starts a background SSH connection that forwards the local
// GPG agent to the remote workspace.
func ForwardAgent(client client2.BaseWorkspaceClient, logger log.Logger) error {
	logger.Debug("gpg forwarding enabled, performing immediately")

	execPath, err := os.Executable()
	if err != nil {
		return err
	}

	remoteUser, err := devssh.GetUser(
		client.WorkspaceConfig().ID,
		client.WorkspaceConfig().SSHConfigPath,
		client.WorkspaceConfig().SSHConfigIncludePath,
	)
	if err != nil {
		remoteUser = "root"
	}

	logger.Info("forwarding gpg-agent")

	args := buildForwardArgs(execPath, remoteUser, client.Context(), client.Workspace())

	go func() {
		err = exec.Command(execPath, args...).Run()
		if err != nil {
			logger.Error("failure in forwarding gpg-agent")
		}
	}()

	return nil
}

func buildForwardArgs(execPath, user, context, workspace string) []string {
	return []string{
		"ssh",
		"--gpg-agent-forwarding=true",
		"--agent-forwarding=true",
		"--start-services=true",
		"--user",
		user,
		"--context",
		context,
		workspace,
		"--log-output=raw",
		"--command", "sleep infinity",
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd pkg/gpg && go test -v ./...`
Expected: PASS

- [ ] **Step 5: Update `pkg/ide/opener/opener.go` to use `pkg/gpg`**

Replace inline `performGpgForwarding` calls in the browser IDE functions with `gpg.ForwardAgent`. Add import `"github.com/skevetter/devpod/pkg/gpg"`. Remove `performGpgForwarding` from wherever it currently lives.

- [ ] **Step 6: Remove `performGpgForwarding` from `cmd/up.go`**

Delete the function (originally lines 1555-1600). Update imports.

- [ ] **Step 7: Verify build and tests pass**

Run: `go build ./... && go test ./pkg/gpg/... ./pkg/ide/opener/... ./cmd/...`
Expected: BUILD OK, PASS

- [ ] **Step 8: Commit**

```bash
git add pkg/gpg/ pkg/ide/opener/ cmd/up.go
git commit -m "$(cat <<'EOF'
refactor: extract pkg/gpg for GPG agent forwarding

Move GPG agent forwarding logic into a dedicated pkg/gpg package.
Update pkg/ide/opener to use it for browser-based IDEs.
EOF
)"
```

---

### Task 4: Move browser tunnel and backhaul into `pkg/tunnel/`

**Files:**
- Create: `pkg/tunnel/browser.go`
- Create: `pkg/tunnel/browser_test.go`
- Modify: `pkg/ide/opener/opener.go` (update to call `tunnel.StartBrowserTunnel`)
- Modify: `cmd/up.go` (remove `startBrowserTunnel`, `setupBackhaul`, `createSSHCommand`)

- [ ] **Step 1: Write failing test**

Create `pkg/tunnel/browser_test.go`:

```go
package tunnel

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestCreateSSHCommandArgs(t *testing.T) {
	tests := []struct {
		name      string
		context   string
		workspace string
		debug     bool
		extraArgs []string
		wantArgs  []string
	}{
		{
			name:      "basic command",
			context:   "default",
			workspace: "my-workspace",
			debug:     false,
			extraArgs: []string{"--stdio"},
			wantArgs: []string{
				"ssh",
				"--user=root",
				"--agent-forwarding=false",
				"--start-services=false",
				"--context",
				"default",
				"my-workspace",
				"--stdio",
			},
		},
		{
			name:      "with debug",
			context:   "default",
			workspace: "my-workspace",
			debug:     true,
			extraArgs: nil,
			wantArgs: []string{
				"ssh",
				"--user=root",
				"--agent-forwarding=false",
				"--start-services=false",
				"--context",
				"default",
				"my-workspace",
				"--debug",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := buildSSHCommandArgs(tt.context, tt.workspace, tt.debug, tt.extraArgs)
			assert.Equal(t, tt.wantArgs, args)
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd pkg/tunnel && go test -v -run TestCreateSSHCommandArgs`
Expected: FAIL -- `buildSSHCommandArgs` undefined

- [ ] **Step 3: Create `pkg/tunnel/browser.go`**

Move from `cmd/up.go`:
- `startBrowserTunnel` -> exported as `StartBrowserTunnel`
- `setupBackhaul` -> exported as `SetupBackhaul`
- `createSSHCommand` -> exported as `CreateSSHCommand` (used by `pkg/ide/opener` for Fleet)
- Extract `buildSSHCommandArgs` as a testable pure function

```go
package tunnel

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/sirupsen/logrus"
	client2 "github.com/skevetter/devpod/pkg/client"
	"github.com/skevetter/devpod/pkg/client/clientimplementation"
	"github.com/skevetter/devpod/pkg/config"
	devssh "github.com/skevetter/devpod/pkg/ssh"
	"github.com/skevetter/log"
	"golang.org/x/crypto/ssh"
)

// BrowserTunnelParams holds parameters for starting a browser-based IDE tunnel.
type BrowserTunnelParams struct {
	DevPodConfig     *config.Config
	Client           client2.BaseWorkspaceClient
	User             string
	TargetURL        string
	ForwardPorts     bool
	ExtraPorts       []string
	AuthSockID       string
	GitSSHSigningKey string
	Log              log.Logger
}

// StartBrowserTunnel starts an SSH tunnel for browser-based IDEs.
func StartBrowserTunnel(ctx context.Context, params BrowserTunnelParams) error {
	// Setup backhaul if authSockID is set
	if params.AuthSockID != "" {
		go func() {
			if err := SetupBackhaul(params.Client, params.AuthSockID, params.Log); err != nil {
				params.Log.Error("Failed to setup backhaul SSH connection: ", err)
			}
		}()
	}

	// handle daemon client directly
	daemonClient, ok := params.Client.(client2.DaemonClient)
	if ok {
		toolClient, _, err := daemonClient.SSHClients(ctx, params.User)
		if err != nil {
			return err
		}
		defer func() { _ = toolClient.Close() }()

		err = clientimplementation.StartServicesDaemon(
			ctx,
			clientimplementation.StartServicesDaemonOptions{
				DevPodConfig: params.DevPodConfig,
				Client:       daemonClient,
				SSHClient:    toolClient,
				User:         params.User,
				Log:          params.Log,
				ForwardPorts: params.ForwardPorts,
				ExtraPorts:   params.ExtraPorts,
			},
		)
		if err != nil {
			return err
		}
		<-ctx.Done()
		return nil
	}

	err := NewTunnel(
		ctx,
		func(ctx context.Context, stdin io.Reader, stdout io.Writer) error {
			writer := params.Log.Writer(logrus.DebugLevel, false)
			defer func() { _ = writer.Close() }()

			cmd, err := CreateSSHCommand(ctx, params.Client, params.Log, []string{
				"--log-output=raw",
				fmt.Sprintf("--reuse-ssh-auth-sock=%s", params.AuthSockID),
				"--stdio",
			})
			if err != nil {
				return err
			}
			cmd.Stdout = stdout
			cmd.Stdin = stdin
			cmd.Stderr = writer
			return cmd.Run()
		},
		func(ctx context.Context, containerClient *ssh.Client) error {
			streamLogger, ok := params.Log.(*log.StreamLogger)
			if ok {
				streamLogger.JSON(logrus.InfoLevel, map[string]string{
					"url":  params.TargetURL,
					"done": "true",
				})
			}

			configureDockerCredentials := params.DevPodConfig.ContextOption(
				config.ContextOptionSSHInjectDockerCredentials,
			) == config.BoolTrue
			configureGitCredentials := params.DevPodConfig.ContextOption(
				config.ContextOptionSSHInjectGitCredentials,
			) == config.BoolTrue
			configureGitSSHSignatureHelper := params.DevPodConfig.ContextOption(
				config.ContextOptionGitSSHSignatureForwarding,
			) == config.BoolTrue

			err := RunServices(
				ctx,
				RunServicesOptions{
					DevPodConfig:                   params.DevPodConfig,
					ContainerClient:                containerClient,
					User:                           params.User,
					ForwardPorts:                   params.ForwardPorts,
					ExtraPorts:                     params.ExtraPorts,
					PlatformOptions:                nil,
					Workspace:                      params.Client.WorkspaceConfig(),
					ConfigureDockerCredentials:     configureDockerCredentials,
					ConfigureGitCredentials:        configureGitCredentials,
					ConfigureGitSSHSignatureHelper: configureGitSSHSignatureHelper,
					GitSSHSigningKey:               params.GitSSHSigningKey,
					Log:                            params.Log,
				},
			)
			if err != nil {
				return fmt.Errorf("run credentials server in browser tunnel: %w", err)
			}

			<-ctx.Done()
			return nil
		},
	)
	return err
}

// SetupBackhaul sets up a long-running SSH connection to keep an AUTH_SOCK alive.
func SetupBackhaul(client client2.BaseWorkspaceClient, authSockID string, logger log.Logger) error {
	execPath, err := os.Executable()
	if err != nil {
		return err
	}

	remoteUser, err := devssh.GetUser(
		client.WorkspaceConfig().ID,
		client.WorkspaceConfig().SSHConfigPath,
		client.WorkspaceConfig().SSHConfigIncludePath,
	)
	if err != nil {
		remoteUser = "root"
	}

	args := []string{
		"ssh",
		"--agent-forwarding=true",
		fmt.Sprintf("--reuse-ssh-auth-sock=%s", authSockID),
		"--start-services=false",
		"--user",
		remoteUser,
		"--context",
		client.Context(),
		client.Workspace(),
		"--log-output=raw",
		"--command",
		"while true; do sleep 6000000; done",
	}

	if logger.GetLevel() == logrus.DebugLevel {
		args = append(args, "--debug")
	}

	logger.Info("Setting up backhaul SSH connection")

	writer := logger.Writer(logrus.InfoLevel, false)
	dotCmd := exec.Command(execPath, args...)
	dotCmd.Stdout = writer
	dotCmd.Stderr = writer

	if err := dotCmd.Run(); err != nil {
		return err
	}

	logger.Infof("Done setting up backhaul")
	return nil
}

// CreateSSHCommand builds an exec.Cmd for SSH into the workspace.
func CreateSSHCommand(
	ctx context.Context,
	client client2.BaseWorkspaceClient,
	logger log.Logger,
	extraArgs []string,
) (*exec.Cmd, error) {
	execPath, err := os.Executable()
	if err != nil {
		return nil, err
	}

	args := buildSSHCommandArgs(
		client.Context(),
		client.Workspace(),
		logger.GetLevel() == logrus.DebugLevel,
		extraArgs,
	)

	return exec.CommandContext(ctx, execPath, args...), nil
}

func buildSSHCommandArgs(context, workspace string, debug bool, extraArgs []string) []string {
	args := []string{
		"ssh",
		"--user=root",
		"--agent-forwarding=false",
		"--start-services=false",
		"--context",
		context,
		workspace,
	}
	if debug {
		args = append(args, "--debug")
	}
	args = append(args, extraArgs...)
	return args
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd pkg/tunnel && go test -v -run TestCreateSSHCommandArgs`
Expected: PASS

- [ ] **Step 5: Update `pkg/ide/opener/opener.go`**

Replace inline `startBrowserTunnel` calls with `tunnel.StartBrowserTunnel`. Replace `createSSHCommand` calls (in `startFleet`) with `tunnel.CreateSSHCommand`. Add import `"github.com/skevetter/devpod/pkg/tunnel"`.

- [ ] **Step 6: Remove from `cmd/up.go`**

Delete: `startBrowserTunnel`, `setupBackhaul`, `createSSHCommand`.
Remove now-unused imports: `"io"`, `"golang.org/x/crypto/ssh"`.

- [ ] **Step 7: Verify build and tests pass**

Run: `go build ./... && go test ./pkg/tunnel/... ./pkg/ide/opener/... ./cmd/...`
Expected: BUILD OK, PASS

- [ ] **Step 8: Commit**

```bash
git add pkg/tunnel/browser.go pkg/tunnel/browser_test.go pkg/ide/opener/ cmd/up.go
git commit -m "$(cat <<'EOF'
refactor: move browser tunnel and backhaul into pkg/tunnel

Move startBrowserTunnel, setupBackhaul, and createSSHCommand from cmd/up.go
into pkg/tunnel where they naturally belong alongside existing tunnel logic.
EOF
)"
```

---

### Task 5: Move provider update check into `pkg/workspace/`

**Files:**
- Modify: `pkg/workspace/provider.go` (add CheckProviderUpdate, GetProInstance)
- Create: `pkg/workspace/provider_update_test.go`
- Modify: `cmd/up.go` (remove `checkProviderUpdate`, `getProInstance`)

- [ ] **Step 1: Write failing tests**

Create `pkg/workspace/provider_update_test.go`:

```go
package workspace

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShouldSkipProviderUpdate(t *testing.T) {
	tests := []struct {
		name           string
		currentVersion string
		isDevVersion   bool
		isInternal     bool
		wantSkip       bool
	}{
		{
			name:         "dev version skips",
			isDevVersion: true,
			wantSkip:     true,
		},
		{
			name:       "internal source skips",
			isInternal: true,
			wantSkip:   true,
		},
		{
			name:           "same version skips",
			currentVersion: "v0.5.0",
			wantSkip:       false, // handled by semver comparison, not this function
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldSkipProviderUpdate(tt.isDevVersion, tt.isInternal)
			assert.Equal(t, tt.wantSkip, got)
		})
	}
}

func TestGetProInstance_NilWhenNoInstances(t *testing.T) {
	// GetProInstance with nil config should return nil gracefully
	result := GetProInstance(nil, "some-provider", nil)
	assert.Nil(t, result)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd pkg/workspace && go test -v -run "TestShouldSkip|TestGetPro"`
Expected: FAIL -- functions not defined

- [ ] **Step 3: Add functions to `pkg/workspace/provider.go`**

Append to existing `pkg/workspace/provider.go`:

```go
// CheckProviderUpdate ensures the provider version matches the pro instance version.
func CheckProviderUpdate(
	devPodConfig *config.Config,
	proInstance *provider2.ProInstance,
	log log.Logger,
) error {
	if version.GetVersion() == version.DevVersion {
		log.Debugf("skipping provider upgrade check during development")
		return nil
	}
	if proInstance == nil {
		log.Debug("no pro instance available, skipping provider upgrade check")
		return nil
	}

	newVersion, err := platform.GetProInstanceDevPodVersion(proInstance)
	if err != nil {
		return fmt.Errorf("version for pro instance %s: %w", proInstance.Host, err)
	}

	p, err := FindProvider(devPodConfig, proInstance.Provider, log)
	if err != nil {
		return fmt.Errorf("get provider config for pro provider %s: %w", proInstance.Provider, err)
	}
	if shouldSkipProviderUpdate(
		p.Config.Version == version.DevVersion,
		p.Config.Source.Internal,
	) {
		return nil
	}

	v1, err := semver.Parse(strings.TrimPrefix(newVersion, "v"))
	if err != nil {
		return fmt.Errorf("parse version %s: %w", newVersion, err)
	}
	v2, err := semver.Parse(strings.TrimPrefix(p.Config.Version, "v"))
	if err != nil {
		return fmt.Errorf("parse version %s: %w", p.Config.Version, err)
	}
	if v1.Compare(v2) == 0 {
		return nil
	}
	log.Infof(
		"New provider version available, attempting to update %s from %s to %s",
		proInstance.Provider,
		p.Config.Version,
		newVersion,
	)

	providerSource, err := ResolveProviderSource(devPodConfig, proInstance.Provider, log)
	if err != nil {
		return fmt.Errorf("resolve provider source %s: %w", proInstance.Provider, err)
	}

	splitted := strings.Split(providerSource, "@")
	if len(splitted) == 0 {
		return fmt.Errorf("no provider source found %s", providerSource)
	}
	providerSource = splitted[0] + "@" + newVersion

	_, err = UpdateProvider(devPodConfig, proInstance.Provider, providerSource, log)
	if err != nil {
		return fmt.Errorf("update provider %s: %w", proInstance.Provider, err)
	}

	log.WithFields(logrus.Fields{
		"provider": proInstance.Provider,
	}).Done("updated provider")
	return nil
}

func shouldSkipProviderUpdate(isDevVersion, isInternal bool) bool {
	return isDevVersion || isInternal
}

// GetProInstance finds the pro instance associated with a provider.
func GetProInstance(
	devPodConfig *config.Config,
	providerName string,
	log log.Logger,
) *provider2.ProInstance {
	if devPodConfig == nil {
		return nil
	}

	proInstances, err := ListProInstances(devPodConfig, log)
	if err != nil {
		return nil
	}
	if len(proInstances) == 0 {
		return nil
	}

	proInstance, ok := FindProviderProInstance(proInstances, providerName)
	if !ok {
		return nil
	}

	return proInstance
}
```

Add necessary imports to `pkg/workspace/provider.go`: `"github.com/blang/semver/v4"`, `"github.com/sirupsen/logrus"`, `"github.com/skevetter/devpod/pkg/platform"`, `"github.com/skevetter/devpod/pkg/version"`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd pkg/workspace && go test -v -run "TestShouldSkip|TestGetPro"`
Expected: PASS

- [ ] **Step 5: Update `cmd/up.go`**

In `prepareClient` method, replace:

```go
// Old:
proInstance := getProInstance(devPodConfig, client.Provider(), logger)
err = checkProviderUpdate(devPodConfig, proInstance, logger)

// New:
proInstance := workspace2.GetProInstance(devPodConfig, client.Provider(), logger)
err = workspace2.CheckProviderUpdate(devPodConfig, proInstance, logger)
```

Remove from `cmd/up.go`: `checkProviderUpdate`, `getProInstance` functions.
Remove now-unused imports: `"github.com/blang/semver/v4"`, `"github.com/skevetter/devpod/pkg/platform"`, `"github.com/skevetter/devpod/pkg/version"` (if no other uses).

- [ ] **Step 6: Verify build and tests pass**

Run: `go build ./... && go test ./pkg/workspace/... ./cmd/...`
Expected: BUILD OK, PASS

- [ ] **Step 7: Commit**

```bash
git add pkg/workspace/provider.go pkg/workspace/provider_update_test.go cmd/up.go
git commit -m "$(cat <<'EOF'
refactor: move provider update check into pkg/workspace

Move checkProviderUpdate and getProInstance from cmd/up.go into
pkg/workspace where they naturally belong alongside FindProvider
and UpdateProvider.
EOF
)"
```

---

### Task 6: Final cleanup of `cmd/up.go`

**Files:**
- Modify: `cmd/up.go`

- [ ] **Step 1: Remove all dead imports**

Run: `go build ./cmd/...`

Fix any unused import errors. The following imports should now be removable from `cmd/up.go` (verify each):
- `"bytes"` (was used by `startFleet`)
- `"io"` (was used by `startBrowserTunnel`)
- `"net"` (was used by `parseAddressAndPort`)
- `"slices"` (was used by dotfiles)
- `"github.com/blang/semver/v4"` (was used by `checkProviderUpdate`)
- Various IDE sub-packages (`fleet`, `jetbrains`, `jupyter`, `openvscode`, `rstudio`, `vscode`, `zed`)
- `"github.com/skevetter/devpod/pkg/command"` (was used by `startFleet`)
- `open2 "github.com/skevetter/devpod/pkg/open"` (was used by browser IDEs)
- `"github.com/skevetter/devpod/pkg/platform"` (was used by `checkProviderUpdate`)
- `"github.com/skevetter/devpod/pkg/port"` (was used by `parseAddressAndPort`)
- `"github.com/skevetter/devpod/pkg/version"` (was used by `checkProviderUpdate`)
- `devssh "github.com/skevetter/devpod/pkg/ssh"` (was used by `createSSHCommand` and `setupDotfiles`)
- `"github.com/skratchdot/open-golang/open"` (was used by `startFleet`)
- `"golang.org/x/crypto/ssh"` (was used by `startBrowserTunnel`)

- [ ] **Step 2: Verify the file is ~350-400 lines**

Run: `wc -l cmd/up.go`
Expected: approximately 350-450 lines

- [ ] **Step 3: Verify full build and all tests pass**

Run: `go build ./... && go test ./...`
Expected: BUILD OK, all PASS

- [ ] **Step 4: Commit**

```bash
git add cmd/up.go
git commit -m "$(cat <<'EOF'
refactor: clean up cmd/up.go after extractions

Remove dead imports and unused code after extracting dotfiles, IDE opener,
GPG forwarding, browser tunnel, and provider update logic into pkg/.
EOF
)"
```

---

## Verification Checklist

After all 6 commits:

- [ ] `go build ./...` passes
- [ ] `go test ./...` passes
- [ ] `go vet ./...` passes
- [ ] `cmd/up.go` is under 450 lines
- [ ] No circular imports (cmd/ -> pkg/, pkg/ -> pkg/, never pkg/ -> cmd/)
- [ ] All new packages have test files
