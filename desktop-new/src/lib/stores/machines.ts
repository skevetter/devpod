import type { UnlistenFn } from "@tauri-apps/api/event"
import { get, writable } from "svelte/store"
import { machineList, machineStatus } from "$lib/ipc/commands.js"
import { onMachinesChanged } from "$lib/ipc/events.js"
import type { Machine } from "$lib/types/index.js"

export const machines = writable<Machine[]>([])
export const machinesLoading = writable(true)

let unlisten: UnlistenFn | null = null
let pollInterval: ReturnType<typeof setInterval> | null = null

const STATUS_POLL_MS = 10_000

export async function initMachines() {
  machinesLoading.set(true)
  try {
    const list = await machineList()
    machines.set(list)
    fetchStatuses(list)
  } catch {
    // Tauri not available
  } finally {
    machinesLoading.set(false)
  }

  try {
    unlisten = await onMachinesChanged((updated) => {
      machines.set(updated)
      fetchStatuses(updated)
    })
  } catch {
    // Event listener setup failed
  }

  pollInterval = setInterval(() => {
    const current = get(machines)
    if (current.length > 0) {
      fetchStatuses(current)
    }
  }, STATUS_POLL_MS)
}

export function destroyMachines() {
  if (unlisten) {
    unlisten()
    unlisten = null
  }
  if (pollInterval) {
    clearInterval(pollInterval)
    pollInterval = null
  }
}

function fetchStatuses(list: Machine[]) {
  for (const m of list) {
    machineStatus(m.id)
      .then((raw) => {
        try {
          const parsed = JSON.parse(raw) as { state?: string }
          if (parsed.state) {
            machines.update((current) =>
              current.map((item) =>
                item.id === m.id ? { ...item, status: parsed.state } : item,
              ),
            )
          }
        } catch {
          const status = raw.trim()
          if (status) {
            machines.update((current) =>
              current.map((item) =>
                item.id === m.id ? { ...item, status } : item,
              ),
            )
          }
        }
      })
      .catch(() => {})
  }
}
