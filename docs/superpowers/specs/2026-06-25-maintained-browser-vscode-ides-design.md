# Spec: Add maintained browser VS Code IDEs (`vscode-web` + `code-server`)

**Issue:** [skevetter/devpod#764](https://github.com/skevetter/devpod/issues/764)
**Date:** 2026-06-25

## Context

The browser IDE `openvscode` (display name "VS Code Browser") downloads
openvscode-server from `github.com/gitpod-io/openvscode-server`, pinned to
`v1.84.2` ([pkg/ide/openvscode/openvscode.go](../../../pkg/ide/openvscode/openvscode.go)).
That upstream is effectively unmaintained, so DevPod's in-browser VS Code is
stuck on an aging build.

This change adds **two actively-maintained browser IDEs alongside** the existing
`openvscode` (which is left untouched for backwards compatibility):

1. **`vscode-web`** — the official VS Code CLI run as `code serve-web`. Always the
   latest VS Code, full Microsoft Marketplace. The CLI binary is small; the server
   bits are fetched at runtime on first connect (needs container network access).
2. **`code-server`** (Coder) — a self-contained tarball from GitHub releases.
   Open VSX marketplace, no runtime download.

Outcome: a user can run an up-to-date VS Code in the browser via
`devpod up --ide=vscode-web` or `--ide=code-server`, and select either from the
desktop app.

## Decisions (and why)

- **Add alongside, don't replace.** `openvscode` keeps working; existing
  `--ide=openvscode` configs and version pins are unaffected. (User decision.)
- **Both backends.** Implement `code serve-web` and `code-server`. (User decision.)
- **Full desktop UI, marked experimental, default ON.** Verified against repo
  convention: the most recent clean single-IDE addition — IBM Bob (#657, commit
  `5c5255d9`) — wired the **full desktop UI** and marked the IDE **experimental**
  with its own settings toggle defaulting to `true` (visible). Every IDE in the
  registry has a desktop icon mapping; there is **no** CLI-only IDE in this repo.
  So CLI-only is not an accepted pattern here — we mirror the Bob convention.
- **Names:** `vscode-web` / `code-server` (the `--ide=` strings). (User decision.)
- **Distinct install dirs.** The regular `vscode` server binary is itself named
  `code-server` and lives in `~/.vscode-server`
  ([vscode.go:62](../../../pkg/ide/vscode/vscode.go#L62)). To avoid any collision
  we use dedicated dirs: `~/.vscode-web` and `~/.code-server`.

## Architecture

Follow the established **one-package-per-IDE** pattern (`openvscode`, `jupyter`,
`rstudio` are each standalone). Create two self-contained packages that mirror
`pkg/ide/openvscode/`, each exposing `Install()`, `Start()`, `InstallExtensions()`,
`Options`, and `getReleaseUrl()`. The intentional duplication keeps each IDE
self-contained and consistent with the codebase. (Go package dirs can't contain
hyphens, so the dirs are `vscodeweb` / `codeserver` while the IDE name strings are
`vscode-web` / `code-server`.)

| Aspect | `vscode-web` (serve-web) | `code-server` |
|---|---|---|
| IDE name (`--ide=`) | `vscode-web` | `code-server` |
| Display name | "VS Code Web" | "code-server" |
| Go package | `pkg/ide/vscodeweb/` | `pkg/ide/codeserver/` |
| Config constant | `IDEVSCodeWeb` | `IDECodeServer` |
| Download (amd64/arm64) | `https://code.visualstudio.com/sha/download?build=stable&os=cli-alpine-{x64,arm64}` | `https://github.com/coder/code-server/releases/download/v{VER}/code-server-{VER}-linux-{amd64,arm64}.tar.gz` |
| Binary after extract (StripLevels 1) | `code` | `bin/code-server` |
| Install dir | `~/.vscode-web` | `~/.code-server` |
| Default `VERSION` option | `stable` | `4.126.0` |
| Start command | `code serve-web --accept-server-license-terms --without-connection-token --host H --port P --server-data-dir <dir>` | `code-server --bind-addr H:P --auth none --user-data-dir <dir> --extensions-dir <dir>` |
| Settings file written | `<data-dir>/Machine/settings.json` | `<user-data-dir>/User/settings.json` |
| Extension install | `code --install-extension <id> --extensions-dir <dir>` | `code-server --install-extension <id> --extensions-dir <dir>` |
| Marketplace | Microsoft | Open VSX |
| Experimental flag | `experimental_vscodeWeb` | `experimental_codeServer` |

Shared mechanics reused from existing code:
- `vscode.InstallAPKRequirements(log)` ([apk.go](../../../pkg/ide/vscode/apk.go)) for
  Alpine/Wolfi (`gcompat` for musl, plus git/bash/curl).
- `command.StartBackgroundOnce`, `extract.Extract(..., StripLevels(1))`,
  `copy2.ChownR`, `devpodhttp.GetHTTPClient()`, `command.GetHome` — same helpers
  openvscode uses.
- Browser exposure via `tunnel.StartBrowserTunnel` and `open2.Open`, mirroring
  `openVSCodeBrowser` in [opener.go](../../../pkg/ide/opener/opener.go).

> **To confirm during implementation** (via `code serve-web --help` and the e2e/manual
> run): the exact data-dir flag set that makes `serve-web` apply Machine settings and
> the pre-installed extensions. `--server-data-dir` is documented; whether
> `--user-data-dir`/`--extensions-dir` are honored by `serve-web` will be verified, and
> the start command adjusted if needed. Settings/extensions are parity-with-openvscode
> niceties; the core acceptance criterion (up-to-date VS Code in browser) does not depend
> on them. `--accept-server-license-terms` is required so the background process never
> blocks on an interactive prompt.

## Touch points

### Go backend (per IDE ×2)
- `pkg/config/ide.go` — add `IDEVSCodeWeb IDE = "vscode-web"` and
  `IDECodeServer IDE = "code-server"`.
- `pkg/ide/vscodeweb/vscodeweb.go` and `pkg/ide/codeserver/codeserver.go` —
  new implementations (mirror `openvscode.go`, with the per-backend specifics above).
- `cmd/agent/container/setup.go` — add `case` in `installIDE()` and a
  `setupVSCodeWeb` / `setupCodeServer` method (mirror `setupOpenVSCode` at
  lines ~593+), plus imports.
- `cmd/agent/container/vscodeweb_async.go` and `codeserver_async.go` — async
  extension-install commands (mirror [openvscode_async.go](../../../cmd/agent/container/openvscode_async.go)).
- `cmd/agent/container/container.go` — register both async commands (mirror line 19).
- `pkg/ide/ideparse/parse.go` — add two `AllowedIDE` entries:
  `Experimental: true`, `Group: config.IDEGroupPrimary`, `Options` from each
  package, `Icon: config.WebsiteAssetsURL + "/vscodebrowser.svg"` (best available
  hosted asset for both), plus imports.
- `pkg/ide/opener/opener.go` — add both names to `browserIDEOpener()` and add
  `openVSCodeWebBrowser` / `openCodeServerBrowser` (mirror `openVSCodeBrowser`).
- `pkg/ide/types.go` — add both names to `ReusesAuthSock()` (return `true`).

### Desktop UI (mirror IBM Bob #657 exactly, ×2)
- `desktop/src/gen/Settings.ts` — add `experimental_vscodeWeb: boolean` and
  `experimental_codeServer: boolean` (file is ts-rs-generated but is hand-edited
  in practice — Bob did the same).
- `desktop/src-tauri/src/settings.rs` — add matching `#[serde(rename = ...)]`
  bool fields. (The Bob PR skipped this, leaving the struct drifted; we sync it
  for correctness.)
- `desktop/src/contexts/SettingsContext/SettingsContext.tsx` — add both keys to
  `initialSettings` defaulting to `true` (visible, matching recent IDEs).
- `desktop/src/views/Settings/Settings.tsx` — add a `Switch` for each in
  `ExperimentalSettings()`.
- `desktop/src/useIDEs.ts` — add a name const + a filter line for each.
- `desktop/src/types.ts` — add both names to `SUPPORTED_IDES`.
- `desktop/src/components/IDEIcon/IDEIcon.tsx` — map both names to an icon.
  Initially reuse `VSCodeBrowser` for both (a distinct `codeserver.svg` is a
  nice-to-have, deferred to avoid fabricating a brand asset).
- `desktop/src/images/index.ts` — only needed if a new SVG is added.

### Tests
- `e2e/tests/ide/ide.go` — add `--ide=vscode-web` and `--ide=code-server`
  `DevPodUpWithIDE` cases next to the existing `openvscode` case (line 39).
- Unit tests for each package's `getReleaseUrl()` (amd64/arm64 + option override),
  the one piece of pure logic worth pinning.

## Non-goals
- Not touching/removing `openvscode`.
- Not adding MS-marketplace override for code-server (Open VSX default).
- Not adding a bespoke `code-server` brand icon in this pass.

## Verification
1. `go build ./...` and `go vet ./...` pass.
2. `golangci-lint` clean on new files.
3. Unit tests: `go test ./pkg/ide/vscodeweb/... ./pkg/ide/codeserver/...`.
4. e2e (docker provider): `devpod up --open-ide=false --ide=vscode-web` and
   `--ide=code-server` succeed against `e2e/tests/ide/testdata`.
5. Manual: `devpod up --ide=vscode-web` / `--ide=code-server`, confirm the browser
   opens an up-to-date VS Code, the workspace folder loads, and (parity check)
   a devcontainer `settings`/`extensions` entry is applied.
6. Desktop: `cd desktop && npm run build` (or type-check) passes; both IDEs appear
   in the picker; their experimental toggles work in Settings.
