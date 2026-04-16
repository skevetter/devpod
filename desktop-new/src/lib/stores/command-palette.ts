import { writable } from "svelte/store"

export interface PaletteItem {
  id: string
  label: string
  description?: string
  category?: string
  href?: string
  action?: () => void
}

export const paletteOpen = writable(false)

export function togglePalette() {
  paletteOpen.update((v) => !v)
}
