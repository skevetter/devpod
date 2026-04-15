import { listen, type UnlistenFn } from "@tauri-apps/api/event"
import type {
  CommandProgress,
  Context,
  Machine,
  Provider,
  Workspace,
} from "$lib/types/index.js"

export const EVENT_NAMES = {
  WORKSPACES_CHANGED: "workspaces-changed",
  PROVIDERS_CHANGED: "providers-changed",
  MACHINES_CHANGED: "machines-changed",
  CONTEXTS_CHANGED: "contexts-changed",
  COMMAND_PROGRESS: "command-progress",
} as const

interface WorkspacesPayload {
  workspaces: Workspace[]
}
interface ProvidersPayload {
  providers: Provider[]
}
interface MachinesPayload {
  machines: Machine[]
}
interface ContextsPayload {
  contexts: Context[]
  activeContext: string
}

export function onWorkspacesChanged(
  callback: (workspaces: Workspace[]) => void,
): Promise<UnlistenFn> {
  return listen<WorkspacesPayload>(EVENT_NAMES.WORKSPACES_CHANGED, (event) => {
    callback(event.payload.workspaces)
  })
}

export function onProvidersChanged(
  callback: (providers: Provider[]) => void,
): Promise<UnlistenFn> {
  return listen<ProvidersPayload>(EVENT_NAMES.PROVIDERS_CHANGED, (event) => {
    callback(event.payload.providers)
  })
}

export function onMachinesChanged(
  callback: (machines: Machine[]) => void,
): Promise<UnlistenFn> {
  return listen<MachinesPayload>(EVENT_NAMES.MACHINES_CHANGED, (event) => {
    callback(event.payload.machines)
  })
}

export function onContextsChanged(
  callback: (contexts: Context[]) => void,
): Promise<UnlistenFn> {
  return listen<ContextsPayload>(EVENT_NAMES.CONTEXTS_CHANGED, (event) => {
    callback(event.payload.contexts)
  })
}

export function onCommandProgress(
  callback: (progress: CommandProgress) => void,
): Promise<UnlistenFn> {
  return listen<CommandProgress>(EVENT_NAMES.COMMAND_PROGRESS, (event) => {
    callback(event.payload)
  })
}
