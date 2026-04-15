import type { UnlistenFn } from "@tauri-apps/api/event"
import { writable } from "svelte/store"
import { providerList } from "$lib/ipc/commands.js"
import { onProvidersChanged } from "$lib/ipc/events.js"
import type { Provider } from "$lib/types/index.js"

export const providers = writable<Provider[]>([])

let unlisten: UnlistenFn | null = null

export async function initProviders() {
  try {
    const list = await providerList()
    providers.set(list)
  } catch {
    // Tauri not available
  }

  try {
    unlisten = await onProvidersChanged((updated) => {
      providers.set(updated)
    })
  } catch {
    // Event listener setup failed
  }
}

export function destroyProviders() {
  if (unlisten) {
    unlisten()
    unlisten = null
  }
}
