import { writable } from "svelte/store"
import { browser } from "$app/environment"

export type Theme = "light" | "dark" | "system"

const STORAGE_KEY = "devpod-theme"

function getInitialTheme(): Theme {
  if (browser) {
    const stored = localStorage.getItem(STORAGE_KEY)
    if (stored === "light" || stored === "dark" || stored === "system") {
      return stored
    }
  }
  return "dark"
}

export const theme = writable<Theme>(getInitialTheme())

export function applyTheme(value: Theme) {
  if (!browser) return

  localStorage.setItem(STORAGE_KEY, value)

  const root = document.documentElement
  if (value === "system") {
    const prefersDark = window.matchMedia(
      "(prefers-color-scheme: dark)",
    ).matches
    root.classList.toggle("dark", prefersDark)
  } else {
    root.classList.toggle("dark", value === "dark")
  }
}

export function cycleTheme() {
  theme.update((current) => {
    const next: Theme =
      current === "light" ? "dark" : current === "dark" ? "system" : "light"
    applyTheme(next)
    return next
  })
}

export function initSettings() {
  const unsubscribe = theme.subscribe((value) => {
    applyTheme(value)
  })

  if (browser) {
    const mediaQuery = window.matchMedia("(prefers-color-scheme: dark)")
    const handler = () => {
      let current: Theme = "dark"
      theme.subscribe((v) => (current = v))()
      if (current === "system") {
        applyTheme("system")
      }
    }
    mediaQuery.addEventListener("change", handler)
    return () => {
      unsubscribe()
      mediaQuery.removeEventListener("change", handler)
    }
  }

  return unsubscribe
}
