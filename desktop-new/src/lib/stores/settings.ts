import { writable } from "svelte/store"
import { browser } from "$app/environment"

// ── UI Settings (localStorage) ──────────────────────────────────────

export type Theme = "light" | "dark" | "system"
export type ColorScheme = "default" | "emerald" | "purple"
export type FontSize = "small" | "medium" | "large"
export type ZoomLevel = "sm" | "md" | "lg" | "xl"
export type SidebarPosition = "left" | "right"

const STORAGE_KEY = "devpod-theme"
const COLOR_SCHEME_KEY = "devpod-color-scheme"
const FONT_SIZE_KEY = "devpod-font-size"
const ZOOM_KEY = "devpod-zoom"
const SIDEBAR_KEY = "devpod-sidebar-position"
const AUTO_UPDATE_KEY = "devpod-auto-update"
const FIXED_IDE_KEY = "devpod-fixed-ide"
const DEFAULT_IDE_KEY = "devpod-default-ide"

const FONT_SIZE_CLASSES: Record<FontSize, string> = {
  small: "text-sm",
  medium: "text-base",
  large: "text-lg",
}

const ZOOM_CLASSES: Record<ZoomLevel, string> = {
  sm: "zoom-sm",
  md: "",
  lg: "zoom-lg",
  xl: "zoom-xl",
}

function getStored<T extends string>(
  key: string,
  valid: readonly T[],
  fallback: T,
): T {
  if (browser) {
    const stored = localStorage.getItem(key)
    if (stored && (valid as readonly string[]).includes(stored)) {
      return stored as T
    }
  }
  return fallback
}

function getStoredBool(key: string, fallback: boolean): boolean {
  if (browser) {
    const stored = localStorage.getItem(key)
    if (stored === "true") return true
    if (stored === "false") return false
  }
  return fallback
}

function getStoredString(key: string, fallback: string): string {
  if (browser) {
    return localStorage.getItem(key) ?? fallback
  }
  return fallback
}

// Theme
export const theme = writable<Theme>(
  getStored(STORAGE_KEY, ["light", "dark", "system"] as const, "dark"),
)

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

// Color scheme (accent)
export const colorScheme = writable<ColorScheme>(
  getStored(
    COLOR_SCHEME_KEY,
    ["default", "emerald", "purple"] as const,
    "default",
  ),
)

const COLOR_SCHEME_CLASSES: ColorScheme[] = ["emerald", "purple"]

export function applyColorScheme(value: ColorScheme) {
  if (!browser) return
  localStorage.setItem(COLOR_SCHEME_KEY, value)
  const root = document.documentElement
  for (const cls of COLOR_SCHEME_CLASSES) {
    root.classList.remove(`theme-${cls}`)
  }
  if (value !== "default") {
    root.classList.add(`theme-${value}`)
  }
}

export function setColorScheme(value: ColorScheme) {
  colorScheme.set(value)
  applyColorScheme(value)
}

// Font size
export const fontSize = writable<FontSize>(
  getStored(FONT_SIZE_KEY, ["small", "medium", "large"] as const, "medium"),
)

export function applyFontSize(value: FontSize) {
  if (!browser) return
  localStorage.setItem(FONT_SIZE_KEY, value)
  const root = document.documentElement
  for (const cls of Object.values(FONT_SIZE_CLASSES)) {
    root.classList.remove(cls)
  }
  root.classList.add(FONT_SIZE_CLASSES[value])
}

// Zoom level
export const zoomLevel = writable<ZoomLevel>(
  getStored(ZOOM_KEY, ["sm", "md", "lg", "xl"] as const, "md"),
)

export function applyZoom(value: ZoomLevel) {
  if (!browser) return
  localStorage.setItem(ZOOM_KEY, value)
  const root = document.documentElement
  for (const cls of Object.values(ZOOM_CLASSES)) {
    if (cls) root.classList.remove(cls)
  }
  const cls = ZOOM_CLASSES[value]
  if (cls) root.classList.add(cls)
}

// Sidebar position
export const sidebarPosition = writable<SidebarPosition>(
  getStored(SIDEBAR_KEY, ["left", "right"] as const, "left"),
)

export function setSidebarPosition(value: SidebarPosition) {
  if (browser) localStorage.setItem(SIDEBAR_KEY, value)
  sidebarPosition.set(value)
}

// Auto-update
export const autoUpdate = writable<boolean>(
  getStoredBool(AUTO_UPDATE_KEY, true),
)

export function setAutoUpdate(value: boolean) {
  if (browser) localStorage.setItem(AUTO_UPDATE_KEY, String(value))
  autoUpdate.set(value)
}

// Default IDE
export const defaultIde = writable<string>(
  getStoredString(DEFAULT_IDE_KEY, "vscode"),
)

export function setDefaultIde(value: string) {
  if (browser) localStorage.setItem(DEFAULT_IDE_KEY, value)
  defaultIde.set(value)
}

// Fixed IDE (always use default)
export const fixedIde = writable<boolean>(getStoredBool(FIXED_IDE_KEY, false))

export function setFixedIde(value: boolean) {
  if (browser) localStorage.setItem(FIXED_IDE_KEY, String(value))
  fixedIde.set(value)
}

// ── Context Options (DevPod CLI) ────────────────────────────────────

export interface ContextOptions {
  debugFlag: boolean
  telemetry: boolean
  agentUrl: string
  dotfilesUrl: string
  sshKeyPath: string
  httpProxy: string
  httpsProxy: string
  noProxy: string
  dockerCredentialForwarding: boolean
  gitCredentialForwarding: boolean
  gitSshSignatureForwarding: boolean
  sshAgentForwarding: boolean
  sshAddPrivateKeys: boolean
  sshStrictHostKeyChecking: boolean
  gpgAgentForwarding: boolean
  additionalCliFlags: string
  additionalEnvVars: string
  experimentalMultiDevcontainer: boolean
}

export const DEFAULT_CONTEXT_OPTIONS: ContextOptions = {
  debugFlag: false,
  telemetry: true,
  agentUrl: "",
  dotfilesUrl: "",
  sshKeyPath: "",
  httpProxy: "",
  httpsProxy: "",
  noProxy: "",
  dockerCredentialForwarding: false,
  gitCredentialForwarding: false,
  gitSshSignatureForwarding: false,
  sshAgentForwarding: false,
  sshAddPrivateKeys: false,
  sshStrictHostKeyChecking: false,
  gpgAgentForwarding: false,
  additionalCliFlags: "",
  additionalEnvVars: "",
  experimentalMultiDevcontainer: false,
}

// Map from our keys to DevPod context option keys
export const CONTEXT_OPTION_KEYS: Record<keyof ContextOptions, string> = {
  debugFlag: "DEBUG",
  telemetry: "TELEMETRY",
  agentUrl: "AGENT_URL",
  dotfilesUrl: "DOTFILES_URL",
  sshKeyPath: "SSH_KEY_PATH",
  httpProxy: "HTTP_PROXY",
  httpsProxy: "HTTPS_PROXY",
  noProxy: "NO_PROXY",
  dockerCredentialForwarding: "DOCKER_CREDENTIAL_HELPER_ENABLED",
  gitCredentialForwarding: "GIT_CREDENTIAL_HELPER_ENABLED",
  gitSshSignatureForwarding: "GIT_SSH_SIGNATURE_FORWARDING_ENABLED",
  sshAgentForwarding: "SSH_AGENT_FORWARDING",
  sshAddPrivateKeys: "SSH_ADD_PRIVATE_KEYS",
  sshStrictHostKeyChecking: "SSH_STRICT_HOST_KEY_CHECKING",
  gpgAgentForwarding: "GPG_AGENT_FORWARDING",
  additionalCliFlags: "ADDITIONAL_FLAGS",
  additionalEnvVars: "ADDITIONAL_ENV_VARS",
  experimentalMultiDevcontainer: "EXPERIMENTAL_MULTI_DEVCONTAINER",
}

export const contextOptions = writable<ContextOptions>({
  ...DEFAULT_CONTEXT_OPTIONS,
})

export function parseContextOptions(
  raw: Record<string, { value?: string }>,
): ContextOptions {
  function str(key: string): string {
    return raw[key]?.value ?? ""
  }
  function bool(key: string): boolean {
    const v = raw[key]?.value
    return v === "true"
  }

  return {
    debugFlag: bool("DEBUG"),
    telemetry: str("TELEMETRY") !== "false",
    agentUrl: str("AGENT_URL"),
    dotfilesUrl: str("DOTFILES_URL"),
    sshKeyPath: str("SSH_KEY_PATH"),
    httpProxy: str("HTTP_PROXY"),
    httpsProxy: str("HTTPS_PROXY"),
    noProxy: str("NO_PROXY"),
    dockerCredentialForwarding: bool("DOCKER_CREDENTIAL_HELPER_ENABLED"),
    gitCredentialForwarding: bool("GIT_CREDENTIAL_HELPER_ENABLED"),
    gitSshSignatureForwarding: bool("GIT_SSH_SIGNATURE_FORWARDING_ENABLED"),
    sshAgentForwarding: bool("SSH_AGENT_FORWARDING"),
    sshAddPrivateKeys: bool("SSH_ADD_PRIVATE_KEYS"),
    sshStrictHostKeyChecking: bool("SSH_STRICT_HOST_KEY_CHECKING"),
    gpgAgentForwarding: bool("GPG_AGENT_FORWARDING"),
    additionalCliFlags: str("ADDITIONAL_FLAGS"),
    additionalEnvVars: str("ADDITIONAL_ENV_VARS"),
    experimentalMultiDevcontainer: bool("EXPERIMENTAL_MULTI_DEVCONTAINER"),
  }
}

// ── Init ────────────────────────────────────────────────────────────

export function initSettings() {
  const unsubTheme = theme.subscribe((value) => {
    applyTheme(value)
  })
  const unsubColor = colorScheme.subscribe((value) => {
    applyColorScheme(value)
  })
  const unsubFont = fontSize.subscribe((value) => {
    applyFontSize(value)
  })
  const unsubZoom = zoomLevel.subscribe((value) => {
    applyZoom(value)
  })
  const unsubscribe = () => {
    unsubTheme()
    unsubColor()
    unsubFont()
    unsubZoom()
  }

  if (browser) {
    const mediaQuery = window.matchMedia("(prefers-color-scheme: dark)")
    const handler = () => {
      let current = "dark" as Theme
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
