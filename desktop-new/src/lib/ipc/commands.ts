import { invoke } from "@tauri-apps/api/core"
import type {
  AuditEntry,
  Context,
  LogEntry,
  Machine,
  OptionValue,
  Provider,
  ProviderOption,
  SshKeyInfo,
  Workspace,
} from "$lib/types/index.js"

// Workspace commands
export async function workspaceList(): Promise<Workspace[]> {
  return invoke<Workspace[]>("workspace_list")
}

export async function workspaceUp(params: {
  source: string
  workspaceId?: string
  provider?: string
  ide?: string
}): Promise<string> {
  return invoke<string>("workspace_up", params)
}

export async function workspaceStop(workspaceId: string): Promise<void> {
  return invoke("workspace_stop", { workspaceId })
}

export async function workspaceDelete(workspaceId: string): Promise<void> {
  return invoke("workspace_delete", { workspaceId })
}

export async function workspaceRebuild(workspaceId: string): Promise<void> {
  return invoke("workspace_rebuild", { workspaceId })
}

export async function workspaceStatus(workspaceId: string): Promise<string> {
  return invoke<string>("workspace_status", { workspaceId })
}

// Provider commands
export async function providerList(): Promise<Provider[]> {
  return invoke<Provider[]>("provider_list")
}

export async function providerAdd(
  name: string,
  source?: string,
): Promise<void> {
  return invoke("provider_add", { name, source })
}

export async function providerDelete(name: string): Promise<void> {
  return invoke("provider_delete", { name })
}

export async function providerUse(name: string): Promise<void> {
  return invoke("provider_use", { name })
}

export async function providerUpdate(name: string): Promise<void> {
  return invoke("provider_update", { name })
}

export async function providerOptions(
  name: string,
): Promise<Record<string, ProviderOption>> {
  return invoke<Record<string, ProviderOption>>("provider_options", { name })
}

export async function providerSetOptions(
  name: string,
  options: Record<string, OptionValue>,
): Promise<void> {
  const optionArgs = Object.entries(options).map(
    ([key, val]) => `${key}=${val}`,
  )
  return invoke("provider_set_options", { name, options: optionArgs })
}

// Machine commands
export async function machineList(): Promise<Machine[]> {
  return invoke<Machine[]>("machine_list")
}

export async function machineCreate(
  name: string,
  provider: string,
  options?: Record<string, OptionValue>,
): Promise<void> {
  return invoke("machine_create", { name, provider, options })
}

export async function machineDelete(
  id: string,
  force?: boolean,
): Promise<void> {
  return invoke("machine_delete", { id, force: force ?? false })
}

export async function machineStart(id: string): Promise<void> {
  return invoke("machine_start", { id })
}

export async function machineStop(id: string): Promise<void> {
  return invoke("machine_stop", { id })
}

export async function machineStatus(id: string): Promise<string> {
  return invoke<string>("machine_status", { id })
}

// Context commands
export async function contextList(): Promise<{
  contexts: Context[]
  activeContext: string
}> {
  return invoke("context_list")
}

export async function contextUse(name: string): Promise<void> {
  return invoke("context_use", { name })
}

// Audit commands
export async function auditRecent(limit?: number): Promise<AuditEntry[]> {
  return invoke<AuditEntry[]>("audit_recent", { limit })
}

export async function auditByResource(
  resourceType: string,
  resourceId: string,
  limit?: number,
): Promise<AuditEntry[]> {
  return invoke<AuditEntry[]>("audit_by_resource", {
    resourceType,
    resourceId,
    limit,
  })
}

// System commands
export async function devpodVersion(): Promise<string> {
  return invoke<string>("devpod_version")
}

// Log commands
export async function workspaceLogsList(
  workspaceId: string,
): Promise<LogEntry[]> {
  return invoke<LogEntry[]>("workspace_logs_list", { workspaceId })
}

export async function workspaceLogRead(
  workspaceId: string,
  filename: string,
): Promise<string> {
  return invoke<string>("workspace_log_read", { workspaceId, filename })
}

// SSH key commands
export async function sshKeyList(): Promise<SshKeyInfo[]> {
  return invoke<SshKeyInfo[]>("ssh_key_list")
}

export async function sshKeyGenerate(params: {
  name: string
  keyType?: string
  comment?: string
}): Promise<SshKeyInfo> {
  return invoke<SshKeyInfo>("ssh_key_generate", params)
}
