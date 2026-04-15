<script lang="ts">
import { terminalCreate, terminalClose } from "$lib/ipc/terminal.js"
import {
  terminals,
  addTerminal,
  removeTerminal,
} from "$lib/stores/terminals.js"
import TerminalComponent from "$lib/components/terminal/Terminal.svelte"
import { Button } from "$lib/components/ui/button/index.js"

let activeSessionId: string | undefined = $state()

async function createShell() {
  try {
    const id = await terminalCreate(80, 24)
    const count = $terminals.filter((t) => t.type === "shell").length + 1
    addTerminal({ id, label: `Shell ${count}`, type: "shell" })
    activeSessionId = id
  } catch (e) {
    console.error("Failed to create terminal:", e)
  }
}

async function closeSession(id: string) {
  try {
    await terminalClose(id)
  } catch {
    // session may already be gone
  }
  removeTerminal(id)
  if (activeSessionId === id) {
    activeSessionId = $terminals.length > 0 ? $terminals[0].id : undefined
  }
}

function handleExit() {
  if (activeSessionId) {
    removeTerminal(activeSessionId)
    activeSessionId = $terminals.length > 0 ? $terminals[0].id : undefined
  }
}
</script>

<div class="flex h-full flex-col">
  <div class="flex items-center justify-between border-b px-4 py-2">
    <h1 class="text-lg font-semibold">Terminals</h1>
    <Button size="sm" on:click={createShell}>New Shell</Button>
  </div>

  {#if $terminals.length > 0}
    <div class="flex gap-1 border-b px-4 py-1">
      {#each $terminals as session (session.id)}
        <button
          class="flex items-center gap-1 rounded px-3 py-1 text-sm transition-colors {activeSessionId ===
          session.id
            ? 'bg-accent text-accent-foreground'
            : 'text-muted-foreground hover:bg-accent/50'}"
          onclick={() => (activeSessionId = session.id)}
        >
          {session.label}
          <span
            role="button"
            tabindex="0"
            class="ml-1 rounded-full px-1 hover:bg-destructive/20"
            onclick={(e) => {
              e.stopPropagation();
              closeSession(session.id);
            }}
            onkeydown={(e) => {
              if (e.key === "Enter") {
                e.stopPropagation();
                closeSession(session.id);
              }
            }}
          >
            x
          </span>
        </button>
      {/each}
    </div>

    <div class="flex-1 overflow-hidden">
      {#each $terminals as session (session.id)}
        {#if session.id === activeSessionId}
          <TerminalComponent sessionId={session.id} onExit={handleExit} />
        {/if}
      {/each}
    </div>
  {:else}
    <div class="flex flex-1 items-center justify-center">
      <div class="text-center text-muted-foreground">
        <p class="mb-2 text-lg">No active terminals</p>
        <Button size="sm" on:click={createShell}>Create a shell</Button>
      </div>
    </div>
  {/if}
</div>
