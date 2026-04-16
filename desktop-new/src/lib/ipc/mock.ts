/**
 * Mock IPC layer for browser-only development.
 * When Tauri is not available (no window.__TAURI_INTERNALS__), this module
 * provides fake invoke() and listen() that return realistic sample data.
 *
 * Usage: `npm run dev` opens http://localhost:1420 in any browser.
 */

import type {
  AuditEntry,
  Context,
  LogEntry,
  Machine,
  Provider,
  ProviderOption,
  SshKeyInfo,
  Workspace,
} from "$lib/types/index.js"

// ── Sample data ──────────────────────────────────────────────────────

const now = new Date().toISOString()
const hourAgo = new Date(Date.now() - 3600_000).toISOString()
const dayAgo = new Date(Date.now() - 86400_000).toISOString()

const WORKSPACES: Workspace[] = [
  {
    id: "my-project",
    uid: "ws-001",
    provider: { name: "docker" },
    ide: { name: "vscode" },
    source: { gitRepository: "github.com/acme/my-project", gitBranch: "main" },
    status: "Running",
    creationTimestamp: dayAgo,
    lastUsedTimestamp: hourAgo,
    context: "default",
  },
  {
    id: "backend-api",
    uid: "ws-002",
    provider: { name: "kubernetes" },
    ide: { name: "goland" },
    source: { gitRepository: "github.com/acme/backend-api" },
    status: "Stopped",
    creationTimestamp: dayAgo,
    lastUsedTimestamp: dayAgo,
    context: "default",
  },
  {
    id: "ml-notebook",
    uid: "ws-003",
    provider: { name: "docker" },
    ide: { name: "jupyternotebook" },
    source: { image: "jupyter/scipy-notebook:latest" },
    status: "Running",
    creationTimestamp: hourAgo,
    lastUsedTimestamp: now,
    context: "default",
  },
]

const PROVIDERS: Provider[] = [
  {
    name: "docker",
    version: "v0.5.0",
    source: { github: "loft-sh/devpod-provider-docker" },
    description: "DevPod on Docker",
    state: { initialized: true, singleMachine: true },
  },
  {
    name: "kubernetes",
    version: "v0.3.2",
    source: { github: "loft-sh/devpod-provider-kubernetes" },
    description: "DevPod on Kubernetes",
    state: { initialized: true, singleMachine: false },
  },
  {
    name: "aws",
    version: "v0.4.1",
    source: { github: "loft-sh/devpod-provider-aws" },
    description: "DevPod on AWS",
    state: { initialized: false, singleMachine: false },
  },
]

const MACHINES: Machine[] = [
  {
    id: "docker-desktop",
    provider: { name: "docker" },
    status: "Running",
    creationTimestamp: dayAgo,
    lastUsedTimestamp: hourAgo,
  },
  {
    id: "k8s-cluster-1",
    provider: { name: "kubernetes" },
    status: "Running",
    creationTimestamp: dayAgo,
    lastUsedTimestamp: now,
  },
]

const CONTEXTS: Context[] = [{ name: "default" }, { name: "staging" }]

const AUDIT_ENTRIES: AuditEntry[] = [
  {
    id: 1,
    timestamp: now,
    action: "start",
    resourceType: "workspace",
    resourceId: "my-project",
    details: "",
    success: true,
  },
  {
    id: 2,
    timestamp: hourAgo,
    action: "stop",
    resourceType: "workspace",
    resourceId: "backend-api",
    details: "",
    success: true,
  },
  {
    id: 3,
    timestamp: hourAgo,
    action: "create",
    resourceType: "workspace",
    resourceId: "ml-notebook",
    details: "",
    success: true,
  },
  {
    id: 4,
    timestamp: dayAgo,
    action: "add",
    resourceType: "provider",
    resourceId: "docker",
    details: "",
    success: true,
  },
  {
    id: 5,
    timestamp: dayAgo,
    action: "add",
    resourceType: "provider",
    resourceId: "kubernetes",
    details: "",
    success: true,
  },
  {
    id: 6,
    timestamp: dayAgo,
    action: "delete",
    resourceType: "workspace",
    resourceId: "old-project",
    details: "forced",
    success: false,
  },
]

const SSH_KEYS: SshKeyInfo[] = [
  {
    name: "id_ed25519",
    keyType: "ed25519",
    fingerprint: "SHA256:abc123def456ghi789jkl012mno345pqr678stu901vwx",
    comment: "user@example.com",
    publicKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI... user@example.com",
    path: "/home/user/.ssh/id_ed25519",
    hasPassphrase: false,
  },
  {
    name: "id_rsa",
    keyType: "rsa",
    fingerprint: "SHA256:xyz789abc012def345ghi678jkl901mno234pqr567stu",
    comment: "deploy@company.com",
    publicKey: "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAAB... deploy@company.com",
    path: "/home/user/.ssh/id_rsa",
    hasPassphrase: true,
  },
]

const PROVIDER_OPTIONS: Record<string, ProviderOption> = {
  DOCKER_HOST: {
    name: "DOCKER_HOST",
    displayName: "Docker Host",
    description: "Docker daemon socket to connect to",
    default: "unix:///var/run/docker.sock",
    value: "unix:///var/run/docker.sock",
    type: "string",
  },
  DISK_SIZE: {
    name: "DISK_SIZE",
    displayName: "Disk Size",
    description: "Size of the disk in GB",
    default: 30,
    value: 30,
    type: "number",
  },
}

const LOG_ENTRIES: LogEntry[] = [
  {
    workspaceId: "my-project",
    filename: "2026-04-16T10-00-00.log",
    createdAt: hourAgo,
    sizeBytes: 4096,
  },
  {
    workspaceId: "my-project",
    filename: "2026-04-15T08-30-00.log",
    createdAt: dayAgo,
    sizeBytes: 2048,
  },
]

// ── Counter for fake IDs ─────────────────────────────────────────────

let counter = 0
function nextId(): string {
  return `mock-${++counter}-${Date.now().toString(36)}`
}

// ── Mock command handlers ────────────────────────────────────────────

type Handler = (args: Record<string, unknown>) => unknown

const COMMANDS: Record<string, Handler> = {
  // Workspaces
  workspace_list: () => WORKSPACES,
  workspace_up: () => nextId(),
  workspace_stop: () => nextId(),
  workspace_delete: () => nextId(),
  workspace_rebuild: () => nextId(),
  workspace_status: (args) => {
    const ws = WORKSPACES.find((w) => w.id === args.workspaceId)
    return ws?.status ?? "NotFound"
  },

  // Providers
  provider_list: () => PROVIDERS,
  provider_add: () => undefined,
  provider_delete: () => undefined,
  provider_use: () => undefined,
  provider_update: () => undefined,
  provider_options: () => PROVIDER_OPTIONS,
  provider_set_options: () => undefined,

  // Machines
  machine_list: () => MACHINES,
  machine_create: () => undefined,
  machine_delete: () => undefined,
  machine_start: () => undefined,
  machine_stop: () => undefined,
  machine_status: (args) => {
    const m = MACHINES.find((machine) => machine.id === args.id)
    return m?.status ?? "Unknown"
  },

  // Contexts
  context_list: () => ({ contexts: CONTEXTS, activeContext: "default" }),
  context_use: () => undefined,

  // Audit
  audit_recent: (args) => {
    const limit = (args.limit as number) ?? 10
    return AUDIT_ENTRIES.slice(0, limit)
  },
  audit_by_resource: (args) =>
    AUDIT_ENTRIES.filter(
      (e) =>
        e.resourceType === args.resourceType &&
        e.resourceId === args.resourceId,
    ),

  // System
  devpod_version: () => "v0.6.0-mock",

  // Logs
  workspace_logs_list: () => LOG_ENTRIES,
  workspace_log_read: () =>
    "2026-04-16 10:00:01 Starting workspace...\n2026-04-16 10:00:03 Pulling image...\n2026-04-16 10:00:15 Container created\n2026-04-16 10:00:16 Workspace ready\n",

  // SSH Keys
  ssh_key_list: () => SSH_KEYS,
  ssh_key_generate: (args) => ({
    name: args.name as string,
    keyType: (args.keyType as string) ?? "ed25519",
    fingerprint: `SHA256:mock-${nextId()}`,
    comment: (args.comment as string) ?? "",
    publicKey: `ssh-ed25519 MOCK... ${args.comment ?? ""}`,
    path: `/home/user/.ssh/${args.name}`,
    hasPassphrase: false,
  }),

  // Terminals
  terminal_create: () => nextId(),
  terminal_create_ssh: () => nextId(),
  terminal_write: () => undefined,
  terminal_resize: () => undefined,
  terminal_close: () => undefined,
  terminal_list: () => [],
}

// ── Mock invoke ──────────────────────────────────────────────────────

export async function invoke<T>(
  cmd: string,
  args?: Record<string, unknown>,
): Promise<T> {
  const handler = COMMANDS[cmd]
  if (!handler) {
    console.warn(`[mock] Unknown command: ${cmd}`)
    throw new Error(`Mock: unknown command "${cmd}"`)
  }

  // Simulate async delay
  await new Promise((r) => setTimeout(r, 50 + Math.random() * 100))

  const result = handler(args ?? {})
  console.debug(`[mock] ${cmd}`, args ?? {}, "→", result)
  return result as T
}

// ── Mock listen ──────────────────────────────────────────────────────

type UnlistenFn = () => void

export async function listen<T>(
  _event: string,
  _callback: (event: { payload: T }) => void,
): Promise<UnlistenFn> {
  // No-op in mock mode — events would come from the Rust backend
  return () => {}
}

// ── Detection ────────────────────────────────────────────────────────

export function isTauri(): boolean {
  return typeof window !== "undefined" && "__TAURI_INTERNALS__" in window
}
