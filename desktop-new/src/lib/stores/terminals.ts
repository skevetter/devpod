import { derived, writable } from "svelte/store"

export interface TerminalSession {
  id: string
  label: string
  type: "shell" | "ssh"
  workspaceId?: string
}

export const terminals = writable<TerminalSession[]>([])

export const terminalCount = derived(terminals, ($t) => $t.length)

export function addTerminal(session: TerminalSession) {
  terminals.update((list) => [...list, session])
}

export function removeTerminal(id: string) {
  terminals.update((list) => list.filter((s) => s.id !== id))
}

export function clearTerminals() {
  terminals.set([])
}
