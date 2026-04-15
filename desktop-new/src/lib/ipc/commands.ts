import { invoke } from "@tauri-apps/api/core"
import type {
  AuditEntry,
  LogEntry,
  Machine,
  OptionValue,
  Provider,
  ProviderOption,
  Workspace,
} from "$lib/types/index.js"

// Workspace commands
export async function workspaceList(): Promise<Workspace[]> {
  return invoke<Workspace[]>("workspace_list")
}

export async function workspaceUp(
  id: string,
  options?: Record<string, OptionValue>,
): Promise<void> {
  return invoke("workspace_up", { id, options })
}

export async function workspaceStop(id: string): Promise<void> {
  return invoke("workspace_stop", { id })
}

export async function workspaceDelete(
  id: string,
  force?: boolean,
): Promise<void> {
  return invoke("workspace_delete", { id, force: force ?? false })
}

export async function workspaceRebuild(id: string): Promise<void> {
  return invoke("workspace_rebuild", { id })
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
  return invoke("provider_set_options", { name, options })
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
