import { invoke } from "@tauri-apps/api/core"
import { listen, type UnlistenFn } from "@tauri-apps/api/event"

export async function terminalCreate(
  cols: number,
  rows: number,
): Promise<string> {
  return invoke<string>("terminal_create", { cols, rows })
}

export async function terminalCreateSsh(
  workspaceId: string,
  cols: number,
  rows: number,
): Promise<string> {
  return invoke<string>("terminal_create_ssh", { workspaceId, cols, rows })
}

export async function terminalWrite(
  sessionId: string,
  data: number[],
): Promise<void> {
  return invoke("terminal_write", { sessionId, data })
}

export async function terminalResize(
  sessionId: string,
  cols: number,
  rows: number,
): Promise<void> {
  return invoke("terminal_resize", { sessionId, cols, rows })
}

export async function terminalClose(sessionId: string): Promise<void> {
  return invoke("terminal_close", { sessionId })
}

export async function terminalListSessions(): Promise<string[]> {
  return invoke<string[]>("terminal_list")
}

interface TerminalOutputPayload {
  session_id: string
  data: number[]
}

interface TerminalExitPayload {
  session_id: string
}

export function onTerminalOutput(
  callback: (sessionId: string, data: Uint8Array) => void,
): Promise<UnlistenFn> {
  return listen<TerminalOutputPayload>("terminal:output", (event) => {
    callback(event.payload.session_id, new Uint8Array(event.payload.data))
  })
}

export function onTerminalExit(
  callback: (sessionId: string) => void,
): Promise<UnlistenFn> {
  return listen<TerminalExitPayload>("terminal:exit", (event) => {
    callback(event.payload.session_id)
  })
}
