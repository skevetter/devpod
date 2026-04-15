import type { UnlistenFn } from "@tauri-apps/api/event"
import { writable } from "svelte/store"
import { machineList } from "$lib/ipc/commands.js"
import { onMachinesChanged } from "$lib/ipc/events.js"
import type { Machine } from "$lib/types/index.js"

export const machines = writable<Machine[]>([])

let unlisten: UnlistenFn | null = null

export async function initMachines() {
  try {
    const list = await machineList()
    machines.set(list)
  } catch {
    // Tauri not available
  }

  try {
    unlisten = await onMachinesChanged((updated) => {
      machines.set(updated)
    })
  } catch {
    // Event listener setup failed
  }
}

export function destroyMachines() {
  if (unlisten) {
    unlisten()
    unlisten = null
  }
}
