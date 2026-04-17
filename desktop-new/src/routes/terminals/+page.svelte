<script lang="ts">
import {
  terminalCreate,
  terminalCreateSsh,
  terminalClose,
} from "$lib/ipc/terminal.js"
import {
  terminals,
  addTerminal,
  removeTerminal,
  renameTerminal,
} from "$lib/stores/terminals.js"
import { workspaces } from "$lib/stores/workspaces.js"
import TerminalComponent from "$lib/components/terminal/Terminal.svelte"
import { Button } from "$lib/components/ui/button/index.js"
import { toasts } from "$lib/stores/toasts.js"
import { onMount } from "svelte"

let activeSessionId: string | undefined = $state()
let showSshMenu = $state(false)
let renamingId: string | undefined = $state()
let renameValue = $state("")

function startRename(id: string, currentLabel: string) {
  renamingId = id
  renameValue = currentLabel
}

function commitRename() {
  if (renamingId && renameValue.trim()) {
    renameTerminal(renamingId, renameValue.trim())
  }
  renamingId = undefined
}

// Auto-select first terminal when navigating to this page
onMount(() => {
  if (!activeSessionId && $terminals.length > 0) {
    activeSessionId = $terminals[0].id
  }
})

let runningWorkspaces = $derived(
  $workspaces.filter((ws) => ws.status?.toLowerCase() === "running"),
)

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

async function createSsh(workspaceId: string) {
  showSshMenu = false
  try {
    const id = await terminalCreateSsh(workspaceId, 80, 24)
    addTerminal({ id, label: `SSH: ${workspaceId}`, type: "ssh", workspaceId })
    activeSessionId = id
    toasts.success(`Connected to ${workspaceId}`)
  } catch (e) {
    toasts.error(`Failed to connect to ${workspaceId}: ${e}`)
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

function closeSshMenu(e: MouseEvent) {
  if (showSshMenu && !(e.target as HTMLElement).closest("[data-ssh-menu]")) {
    showSshMenu = false
  }
}

function handleExit() {
  if (activeSessionId) {
    removeTerminal(activeSessionId)
    activeSessionId = $terminals.length > 0 ? $terminals[0].id : undefined
  }
}
</script>

<!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
<div class="flex h-full flex-col" role="presentation" onclick={closeSshMenu}>
  <div class="flex items-center justify-between border-b px-4 py-2">
    <h1 class="text-lg font-semibold">Terminals</h1>
    <div class="flex gap-2">
      <div class="relative" data-ssh-menu>
        <Button
          variant="outline"
          size="sm"
          onclick={() => (showSshMenu = !showSshMenu)}
          disabled={runningWorkspaces.length === 0}
        >
          SSH into Workspace
        </Button>
        {#if showSshMenu && runningWorkspaces.length > 0}
          <div class="absolute right-0 top-full z-10 mt-1 w-56 rounded-md border bg-popover p-1 shadow-md">
            {#each runningWorkspaces as ws (ws.id)}
              <button
                class="w-full rounded-sm px-3 py-2 text-left text-sm hover:bg-accent"
                onclick={() => createSsh(ws.id)}
              >
                {ws.id}
              </button>
            {/each}
          </div>
        {/if}
      </div>
      <Button size="sm" onclick={createShell}>New Shell</Button>
    </div>
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
          ondblclick={() => startRename(session.id, session.label)}
        >
          {#if renamingId === session.id}
            <!-- svelte-ignore a11y_autofocus -->
            <input
              class="w-20 rounded border bg-background px-1 text-sm"
              bind:value={renameValue}
              autofocus
              onblur={commitRename}
              onkeydown={(e) => {
                if (e.key === "Enter") commitRename()
                if (e.key === "Escape") { renamingId = undefined }
              }}
              onclick={(e) => e.stopPropagation()}
            />
          {:else}
            {session.label}
          {/if}
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
        <div class="h-full" class:hidden={session.id !== activeSessionId}>
          <TerminalComponent sessionId={session.id} active={session.id === activeSessionId} onExit={handleExit} />
        </div>
      {/each}
    </div>
  {:else}
    <div class="flex flex-1 items-center justify-center">
      <div class="text-center text-muted-foreground">
        <p class="mb-2 text-lg">No active terminals</p>
        <div class="flex gap-2 justify-center">
          <Button size="sm" onclick={createShell}>New Shell</Button>
          {#if runningWorkspaces.length > 0}
            <div class="relative" data-ssh-menu>
              <Button variant="outline" size="sm" onclick={() => (showSshMenu = !showSshMenu)}>
                SSH into Workspace
              </Button>
              {#if showSshMenu}
                <div class="absolute left-1/2 -translate-x-1/2 top-full z-10 mt-1 w-56 rounded-md border bg-popover p-1 shadow-md">
                  {#each runningWorkspaces as ws (ws.id)}
                    <button
                      class="w-full rounded-sm px-3 py-2 text-left text-sm hover:bg-accent"
                      onclick={() => createSsh(ws.id)}
                    >
                      {ws.id}
                    </button>
                  {/each}
                </div>
              {/if}
            </div>
          {/if}
        </div>
      </div>
    </div>
  {/if}
</div>
