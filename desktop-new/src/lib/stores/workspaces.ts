import type { UnlistenFn } from "@tauri-apps/api/event"
import { get, writable } from "svelte/store"
import { workspaceList, workspaceStatus } from "$lib/ipc/commands.js"
import { onWorkspacesChanged } from "$lib/ipc/events.js"
import type { Workspace } from "$lib/types/index.js"

export const workspaces = writable<Workspace[]>([])
export const workspacesLoading = writable(true)

let unlisten: UnlistenFn | null = null
let pollInterval: ReturnType<typeof setInterval> | null = null

const STATUS_POLL_MS = 10_000

export async function initWorkspaces() {
  workspacesLoading.set(true)
  try {
    const list = await workspaceList()
    workspaces.set(list)
    fetchStatuses(list)
  } catch {
    // Tauri not available (e.g. during SSR or browser preview)
  } finally {
    workspacesLoading.set(false)
  }

  try {
    unlisten = await onWorkspacesChanged((updated) => {
      workspaces.set(updated)
      fetchStatuses(updated)
    })
  } catch {
    // Event listener setup failed
  }

  // Poll statuses periodically to keep dashboard and badges fresh
  pollInterval = setInterval(() => {
    const current = get(workspaces)
    if (current.length > 0) {
      fetchStatuses(current)
    }
  }, STATUS_POLL_MS)
}

export function destroyWorkspaces() {
  if (unlisten) {
    unlisten()
    unlisten = null
  }
  if (pollInterval) {
    clearInterval(pollInterval)
    pollInterval = null
  }
}

/** Fetch status for each workspace and merge into store */
function fetchStatuses(list: Workspace[]) {
  for (const ws of list) {
    workspaceStatus(ws.id)
      .then((raw) => {
        try {
          const parsed = JSON.parse(raw) as { state?: string }
          if (parsed.state) {
            workspaces.update((current) =>
              current.map((w) =>
                w.id === ws.id ? { ...w, status: parsed.state } : w,
              ),
            )
          }
        } catch {
          // Status response wasn't valid JSON — use raw as status
          const status = raw.trim()
          if (status) {
            workspaces.update((current) =>
              current.map((w) => (w.id === ws.id ? { ...w, status } : w)),
            )
          }
        }
      })
      .catch(() => {
        // Status fetch failed — leave as-is
      })
  }
}
