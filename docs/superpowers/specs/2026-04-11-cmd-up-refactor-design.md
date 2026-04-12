# Design: Extract cmd/up.go into pkg/ packages

**Date:** 2026-04-11
**Status:** Approved
**Approach:** Single PR, one commit per extracted package (Approach C)

## Problem

`cmd/up.go` is 1839 lines with 6 distinct responsibility areas packed into one file:
command definition, client preparation, devPodUp orchestration, IDE opening,
SSH/tunnel/credentials setup, and dotfiles/utilities. This makes it hard to
navigate, test, and modify.

## Goal

Break `cmd/up.go` into focused `pkg/` packages, each with unit tests. The file
should shrink to ~350-400 lines of command-level orchestration. All behavior must
be preserved.

## Package Extractions

### 1. `pkg/dotfiles/` (new)

**Source lines:** 1384-1553
**Functions moved:** `setupDotfiles`, `buildDotCmd`, `buildDotCmdAgentArguments`,
`extractKeysFromEnvKeyValuePairs`, `collectDotfilesScriptEnvKeyvaluePairs`

**Exported API:**

```go
package dotfiles

type SetupParams struct {
    Source       string
    Script       string
    EnvFiles     []string
    EnvKeyValues []string
    Client       client.BaseWorkspaceClient
    DevPodConfig *config.Config
    Log          log.Logger
}

func Setup(params SetupParams) error
```

**Tests:** Table-driven tests for `extractKeysFromEnvKeyValuePairs`,
`collectDotfilesScriptEnvKeyvaluePairs`, and `buildDotCmdAgentArguments`
(pure functions). Integration-style test for `Setup` with a mock client.

### 2. `pkg/ide/opener/` (new)

**Source lines:** 400-583, 840-1056
**Functions moved:** `ideOpener` struct and methods, `openVSCodeFlavor`,
`openJetBrains`, `startVSCodeInBrowser`, `startJupyterNotebookInBrowser`,
`startRStudioInBrowser`, `startFleet`, `parseAddressAndPort`

**Exported API:**

```go
package opener

type Params struct {
    GPGAgentForwarding bool
    SSHAuthSockID      string
    GitSSHSigningKey   string
    DevPodConfig       *config.Config
    Client             client.BaseWorkspaceClient
    User               string
    Workdir            string
    Result             *config2.Result
    Log                log.Logger
}

func Open(ctx context.Context, ideName string, ideOptions map[string]config.OptionValue, params Params) error
```

**Tests:** `parseAddressAndPort` table-driven tests. IDE dispatch routing test
(verify correct branch taken per IDE name).

### 3. `pkg/gpg/` (new)

**Source lines:** 1555-1600
**Functions moved:** `performGpgForwarding`

**Exported API:**

```go
package gpg

func ForwardAgent(client client.BaseWorkspaceClient, log log.Logger) error
```

**Tests:** Verify command construction arguments.

### 4. Extend `pkg/tunnel/` (browser tunnel + backhaul)

**Source lines:** 1089-1263
**Functions moved:** `startBrowserTunnel`, `setupBackhaul`, `createSSHCommand`
(internal helper)

**New exports:**

```go
// In pkg/tunnel/

type BrowserTunnelParams struct {
    DevPodConfig     *config.Config
    Client           client.BaseWorkspaceClient
    User             string
    TargetURL        string
    ForwardPorts     bool
    ExtraPorts       []string
    AuthSockID       string
    GitSSHSigningKey string
    Log              log.Logger
}

func StartBrowserTunnel(ctx context.Context, params BrowserTunnelParams) error

func SetupBackhaul(client client.BaseWorkspaceClient, authSockID string, log log.Logger) error
```

**Tests:** Unit test for SSH command argument construction.

### 5. Move provider update check to `pkg/workspace/`

**Source lines:** 1602-1693
**Functions moved:** `checkProviderUpdate`, `getProInstance`

**New exports:**

```go
// In pkg/workspace/

func CheckProviderUpdate(devPodConfig *config.Config, proInstance *provider2.ProInstance, log log.Logger) error

func GetProInstance(devPodConfig *config.Config, providerName string, log log.Logger) *provider2.ProInstance
```

**Tests:** Table-driven test for version comparison logic (dev version skip,
same version skip, upgrade path).

## What stays in cmd/up.go (~350-400 lines)

- `UpCmd` struct, `NewUpCmd`, flag registration methods (~180 lines)
- `execute`, `validate`, `prepareClient` (command orchestration)
- `Run`, `prepareWorkspace`, `executeDevPodUp`, `configureWorkspace`, `openIDE`
  (thin orchestration calling into pkg/)
- `devPodUp`, `devPodUpMachine`, `devPodUpProxy`, `devPodUpDaemon` (client
  dispatch, tightly coupled to cobra context)
- `WithSignals`, `validatePodmanFlags`, `isValidMapping` (command utilities)
- `mergeDevPodUpOptions`, `mergeEnvFromFiles` (option merging)

## Commit Sequence

1. `refactor: extract pkg/dotfiles for dotfiles setup`
2. `refactor: extract pkg/ide/opener for IDE launch dispatch`
3. `refactor: extract pkg/gpg for GPG agent forwarding`
4. `refactor: move browser tunnel and backhaul into pkg/tunnel`
5. `refactor: move provider update check into pkg/workspace`
6. `refactor: clean up cmd/up.go after extractions`

Each commit moves a responsibility area out, adds tests, and updates
`cmd/up.go` to call the new package. No intermediate broken state.

## Cross-cutting Dependencies

`createSSHCommand` is used by `startBrowserTunnel`, `setupBackhaul`, and
`startFleet` (IDE opener). It moves to `pkg/tunnel/` as an internal helper,
and `pkg/ide/opener/` imports `pkg/tunnel/` to reuse it (or it gets promoted
to an exported function in `pkg/tunnel/`). Direction is still pkg/ -> pkg/,
no circular risk.

## Risks and Mitigations

- **Circular imports:** The extracted packages depend on `pkg/client`,
  `pkg/config`, `pkg/ssh` etc. but not on `cmd/`. Direction is always
  cmd/ -> pkg/, never reverse. No circular risk.
- **Behavior regression:** Pure refactor with no logic changes. Existing
  integration tests plus new unit tests provide coverage.
- **Large PR:** Mitigated by clean per-commit structure. Each commit is
  independently reviewable.
