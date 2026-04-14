# DevPod UI Rebuild — Design Spec

**Date:** 2026-04-14
**Status:** Approved
**Scope:** Complete UI rebuild replacing React/Chakra desktop app with Svelte/shadcn + rewritten Rust backend

## Summary

Replace the existing Tauri v2 + React + Chakra UI desktop application with a clean-slate Tauri v2 + Svelte + shadcn-svelte + Tailwind CSS v4 application. The new UI provides full management of the open-source devpod CLI (no Pro/Loft integration). The Rust backend is rewritten as a persistent daemon with internal API, real-time state management, and embedded terminal support.

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Desktop shell | Tauri v2 | Keep native cross-platform; already proven in this project |
| Frontend framework | Svelte + SvelteKit (static adapter) | Modern, less boilerplate than React, native reactivity |
| Component library | shadcn-svelte + Tailwind CSS v4 | Utility-first, composable, no runtime CSS-in-JS overhead |
| CLI communication | Rust daemon with internal API | Richest UX: real-time updates, offline state, push events |
| Pro/Loft integration | Dropped | Focus on open-source devpod CLI management only |
| Terminal | Embedded xterm.js via PTY manager | Full SSH sessions and live command output in-app |
| Platforms | macOS, Linux, Windows | All three from day one |
| System tray | Minimal | Show/hide window only, no workspace management |
| Persistence | Workspace logs + SQLite audit log | Customer-facing log history + developer event auditing |

## Architecture

```
┌─────────────────────────────────────────────┐
│  Svelte Frontend (shadcn-svelte + Tailwind) │
│  ┌──────────┐ ┌──────────┐ ┌─────────────┐ │
│  │ Views    │ │ Stores   │ │ xterm.js    │ │
│  │ (pages)  │ │ (state)  │ │ (terminal)  │ │
│  └────┬─────┘ └────┬─────┘ └──────┬──────┘ │
│       └─────┬──────┘              │         │
│         invoke()            events/streams  │
├─────────────┼─────────────────────┼─────────┤
│  Rust Backend (Tauri v2)          │         │
│  ┌──────────┴──────────┐  ┌──────┴──────┐  │
│  │ Tauri Commands      │  │ Event Bus   │  │
│  │ (typed IPC)         │  │ (push to FE)│  │
│  └──────────┬──────────┘  └──────┬──────┘  │
│         ┌───┴───┐                │          │
│  ┌──────┴──────┐  ┌─────────────┴───────┐  │
│  │ Daemon      │  │ PTY Manager         │  │
│  │ (state mgr, │  │ (terminal sessions  │  │
│  │  CLI runner, │  │  via portable-pty)  │  │
│  │  file watch, │  └───────────────────┘  │
│  │  audit log) │                          │
│  └─────────────┘                          │
└─────────────────────────────────────────────┘
         │
         ▼
   devpod CLI binary
```

## Frontend Structure

```
desktop-new/
├── src/
│   ├── lib/
│   │   ├── components/
│   │   │   ├── ui/              # shadcn-svelte primitives
│   │   │   ├── layout/          # Shell, Sidebar, TopBar
│   │   │   ├── terminal/        # xterm.js wrapper
│   │   │   ├── workspace/       # workspace cards, forms, detail
│   │   │   ├── provider/        # provider cards, config forms
│   │   │   └── machine/         # machine list, detail
│   │   ├── stores/
│   │   │   ├── workspaces.ts    # workspace state from daemon
│   │   │   ├── providers.ts     # provider state
│   │   │   ├── machines.ts      # machine state
│   │   │   ├── terminals.ts     # active terminal sessions
│   │   │   └── settings.ts      # app settings
│   │   ├── ipc/
│   │   │   ├── commands.ts      # typed invoke() wrappers
│   │   │   └── events.ts        # typed event listeners
│   │   └── types/
│   │       └── index.ts         # shared types matching Rust
│   ├── routes/
│   │   ├── +layout.svelte       # app shell with sidebar
│   │   ├── workspaces/
│   │   │   ├── +page.svelte     # workspace list
│   │   │   ├── [id]/
│   │   │   │   └── +page.svelte # workspace detail + terminal
│   │   │   └── new/
│   │   │       └── +page.svelte # create workspace
│   │   ├── providers/
│   │   │   ├── +page.svelte     # provider list
│   │   │   ├── [id]/
│   │   │   │   └── +page.svelte # provider detail + options
│   │   │   └── add/
│   │   │       └── +page.svelte # add provider
│   │   ├── machines/
│   │   │   ├── +page.svelte     # machine list
│   │   │   └── [id]/
│   │   │       └── +page.svelte # machine detail
│   │   ├── terminals/
│   │   │   └── +page.svelte     # terminal manager
│   │   └── settings/
│   │       └── +page.svelte     # app settings, contexts, IDEs
│   └── app.html
├── src-tauri/
│   └── src/
│       ├── daemon/
│       │   ├── mod.rs
│       │   ├── state.rs         # in-memory state store
│       │   ├── watcher.rs       # polls CLI, detects changes
│       │   └── cli.rs           # CLI subprocess runner
│       ├── commands/
│       │   ├── mod.rs
│       │   ├── workspaces.rs    # workspace CRUD commands
│       │   ├── providers.rs     # provider CRUD commands
│       │   ├── machines.rs      # machine CRUD commands
│       │   ├── contexts.rs      # context management
│       │   ├── ides.rs          # IDE list/config
│       │   └── settings.rs      # app settings
│       ├── terminal/
│       │   ├── mod.rs
│       │   └── pty.rs           # PTY session management
│       ├── persistence/
│       │   ├── mod.rs
│       │   ├── logs.rs          # workspace log file storage
│       │   └── audit.rs         # SQLite audit log
│       ├── events.rs            # typed event definitions
│       └── main.rs
├── tailwind.config.ts
├── svelte.config.js
├── vite.config.ts
├── package.json
└── Cargo.toml
```

### Routing

SvelteKit with `@sveltejs/adapter-static` for Tauri. File-based routing, fully client-side (no SSR).

### State Management

Svelte stores subscribe to daemon events. On mount, stores call the corresponding Tauri command to get initial state, then listen for change events to stay in sync. No polling from the frontend — the daemon pushes all updates.

### IPC Layer

Thin typed wrappers in `lib/ipc/` around `invoke()` and `listen()`. These are the only files that import from `@tauri-apps/api`. All other code goes through stores or the IPC layer.

## Views & Feature Map

### Workspaces (primary view)

- **List** — cards/table showing name, provider, status (running/stopped/busy), IDE, source, last used. Real-time status from daemon watcher.
- **Create** — form: source (git repo URL, local path, or container image), provider select, IDE select, workspace name. Maps to `devpod up`.
- **Detail** — status, logs tab (historical + live), terminal tab (SSH session via PTY), start/stop/rebuild/delete actions. Maps to `devpod up/stop/delete/ssh/logs`.
- **Import/Export** — `devpod import`/`devpod export` for workspace portability.

### Providers

- **List** — installed providers with status, version, default marker.
- **Add** — search/browse available providers, install with `devpod provider add`.
- **Detail** — options form (dynamically generated from `devpod provider options`), update, set as default, rename, delete.

### Machines

- **List** — machines with provider, status, creation time.
- **Detail** — inspect, start/stop/delete, SSH session via PTY.

### Terminals

- **Manager** — tabbed terminal sessions. New sessions from workspace SSH or standalone. Persistent across view navigation (sessions live in PTY manager, not tied to route).
- Active terminal count shown as badge on Terminals sidebar item.

### Settings

- **Contexts** — list/create/delete/switch contexts, set context options.
- **IDEs** — configure default IDE, list supported IDEs.
- **App** — theme (light/dark/system), CLI binary path, telemetry opt-out.

### Sidebar Navigation

Workspaces, Providers, Machines, Terminals (with badge), Settings.

## Rust Daemon Design

### State Store (`daemon/state.rs`)

```rust
struct DaemonState {
    workspaces: HashMap<String, Workspace>,
    providers: HashMap<String, Provider>,
    machines: HashMap<String, Machine>,
    contexts: Vec<Context>,
    active_context: String,
}
```

All state lives in `Arc<RwLock<DaemonState>>` shared across Tauri commands.

### Watcher (`daemon/watcher.rs`)

- Background `tokio` task polling `devpod list --output json`, `devpod provider list --output json`, `devpod machine list --output json` on configurable interval (default 3s).
- Diffs results against cached state. Only emits events on actual changes.
- Also watches `~/.devpod/` via `notify` crate for file changes as secondary trigger (immediate detection of manual CLI usage).

### CLI Runner (`daemon/cli.rs`)

- Async subprocess execution via `tokio::process::Command`.
- Two modes:
  - **Fire-and-wait** — returns parsed JSON result.
  - **Streaming** — pipes stdout/stderr line-by-line to event channel for log viewing.
- Mutation commands (up, stop, delete) acquire per-resource lock to prevent concurrent conflicting operations. Frontend shows "busy" state.

### PTY Manager (`terminal/pty.rs`)

- `portable-pty` crate for cross-platform PTY support.
- Sessions tracked by UUID. Frontend sends input via Tauri command, receives output via events.
- Resize events: xterm.js → Tauri command → PTY.
- Sessions survive view navigation. Explicit close or window close terminates them.

### Event Types (`events.rs`)

```rust
WorkspacesChanged(Vec<Workspace>)
ProvidersChanged(Vec<Provider>)
MachinesChanged(Vec<Machine>)
TerminalOutput { session_id: String, data: Vec<u8> }
CommandProgress { id: String, status: String, output_line: String }
```

## Persistence Layer

### Workspace Log Storage (`persistence/logs.rs`)

- Each workspace operation (up, stop, delete, rebuild) writes full stdout/stderr to `~/.devpod/logs/{workspace_id}/{timestamp}-{command}.log`.
- Daemon indexes these in state for frontend listing.
- Workspace detail view shows **Logs** tab with historical operations — click any entry to view full output.
- Configurable retention (default: 30 days). Old logs pruned by watcher on startup.

### SQLite Audit Log (`persistence/audit.rs`)

- Stored at `~/.devpod/audit.db` using `rusqlite` crate.
- Schema:
  ```sql
  CREATE TABLE events (
      id          INTEGER PRIMARY KEY AUTOINCREMENT,
      timestamp   TEXT NOT NULL,       -- ISO 8601
      event_type  TEXT NOT NULL,       -- workspace_created, provider_added, etc.
      resource_id TEXT,                -- workspace/provider/machine ID
      command     TEXT,                -- CLI command that was run
      payload     TEXT,                -- JSON blob: state diff, error details
      duration_ms INTEGER,            -- for completed operations
      status      TEXT NOT NULL        -- success, error, timeout
  );
  ```
- Every daemon event, CLI invocation, and state change gets a row.
- WAL mode for concurrent reads during writes.
- Development/debugging tool — not exposed in customer-facing UI, queryable directly.

## System Tray

Minimal tray icon. Only actions: show window, hide window, quit. No workspace management in tray.

## Key Dependencies

### Frontend (npm)
- `svelte` + `@sveltejs/kit` + `@sveltejs/adapter-static`
- `shadcn-svelte`
- `tailwindcss` v4
- `@tauri-apps/api` v2 + plugins (shell, fs, dialog, os, process, store, updater, clipboard, log)
- `@xterm/xterm` + `@xterm/addon-fit`

### Backend (Cargo)
- `tauri` v2 + plugins
- `tokio` (async runtime)
- `serde` + `serde_json` (serialization)
- `portable-pty` (cross-platform PTY)
- `notify` (filesystem watching)
- `rusqlite` (audit log)

## Development Process Constraint

All implementation agents must have explicit deadlines. If an agent hasn't completed by its deadline, it terminates and the task gets rescheduled or escalated.
