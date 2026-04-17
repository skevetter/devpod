<script lang="ts">
import { onMount, onDestroy } from "svelte"
import type { Terminal } from "@xterm/xterm"
import type { FitAddon } from "@xterm/addon-fit"
import {
  terminalWrite,
  terminalResize,
  onTerminalOutput,
  onTerminalExit,
} from "$lib/ipc/terminal.js"
import { theme } from "$lib/stores/settings.js"
import { get } from "svelte/store"

let {
  sessionId,
  active = true,
  onExit,
}: { sessionId: string; active?: boolean; onExit?: () => void } = $props()

let containerEl: HTMLDivElement | undefined = $state()

let term: Terminal | undefined
let fitAddon: FitAddon | undefined
let unlistenOutput: (() => void) | undefined
let unlistenExit: (() => void) | undefined
let unsubscribeTheme: (() => void) | undefined
let resizeObserver: ResizeObserver | undefined

const darkTheme = {
  background: "#1e1e2e",
  foreground: "#cdd6f4",
  cursor: "#f5e0dc",
  selectionBackground: "#585b70",
}

const lightTheme = {
  background: "#eff1f5",
  foreground: "#4c4f69",
  cursor: "#dc8a78",
  selectionBackground: "#ccd0da",
}

function isDark(): boolean {
  const current = get(theme)
  if (current === "system") {
    return window.matchMedia("(prefers-color-scheme: dark)").matches
  }
  return current === "dark"
}

onMount(async () => {
  if (!containerEl) return

  // Dynamic imports — xterm requires DOM APIs not available during SSR
  const [{ Terminal: XTerm }, { FitAddon: XFitAddon }] = await Promise.all([
    import("@xterm/xterm"),
    import("@xterm/addon-fit"),
  ])
  await import("@xterm/xterm/css/xterm.css")

  term = new XTerm({
    cursorBlink: true,
    fontSize: 14,
    fontFamily: "monospace",
    theme: isDark() ? darkTheme : lightTheme,
  })

  unsubscribeTheme = theme.subscribe(() => {
    if (term) {
      term.options.theme = isDark() ? darkTheme : lightTheme
    }
  })

  fitAddon = new XFitAddon()
  term.loadAddon(fitAddon)
  term.open(containerEl)
  fitAddon.fit()

  term.onData((data) => {
    const encoded = new TextEncoder().encode(data)
    terminalWrite(sessionId, Array.from(encoded))
  })

  unlistenOutput = await onTerminalOutput((sid, data) => {
    if (sid === sessionId && term) {
      term.write(data)
    }
  })

  unlistenExit = await onTerminalExit((sid) => {
    if (sid === sessionId) {
      onExit?.()
    }
  })

  resizeObserver = new ResizeObserver(() => {
    if (fitAddon && term) {
      fitAddon.fit()
      terminalResize(sessionId, term.cols, term.rows)
    }
  })
  resizeObserver.observe(containerEl)
})

// Refit and focus when tab becomes active
$effect(() => {
  if (active && fitAddon && term) {
    // Use requestAnimationFrame to ensure the container is visible before fitting
    requestAnimationFrame(() => {
      fitAddon?.fit()
      term?.focus()
    })
  }
})

onDestroy(() => {
  resizeObserver?.disconnect()
  unlistenOutput?.()
  unlistenExit?.()
  unsubscribeTheme?.()
  term?.dispose()
})
</script>

<div bind:this={containerEl} class="h-full w-full p-2"></div>
