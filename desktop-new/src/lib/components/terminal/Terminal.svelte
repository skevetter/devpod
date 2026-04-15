<script lang="ts">
import { onMount, onDestroy } from "svelte"
import { Terminal } from "@xterm/xterm"
import { FitAddon } from "@xterm/addon-fit"
import "@xterm/xterm/css/xterm.css"
import {
  terminalWrite,
  terminalResize,
  onTerminalOutput,
  onTerminalExit,
} from "$lib/ipc/terminal.js"

let { sessionId, onExit }: { sessionId: string; onExit?: () => void } = $props()

let containerEl: HTMLDivElement | undefined = $state()

let term: Terminal | undefined
let fitAddon: FitAddon | undefined
let unlistenOutput: (() => void) | undefined
let unlistenExit: (() => void) | undefined
let resizeObserver: ResizeObserver | undefined

onMount(async () => {
  if (!containerEl) return

  term = new Terminal({
    cursorBlink: true,
    fontSize: 14,
    fontFamily: "monospace",
    theme: {
      background: "#1e1e2e",
      foreground: "#cdd6f4",
    },
  })

  fitAddon = new FitAddon()
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

onDestroy(() => {
  resizeObserver?.disconnect()
  unlistenOutput?.()
  unlistenExit?.()
  term?.dispose()
})
</script>

<div bind:this={containerEl} class="h-full w-full"></div>
