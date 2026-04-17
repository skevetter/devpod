<script lang="ts">
import {
  Plus,
  Terminal as TerminalIcon,
  Monitor,
  X,
  SquareTerminal,
} from "@lucide/svelte"
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
import { Input } from "$lib/components/ui/input/index.js"
import * as DropdownMenu from "$lib/components/ui/dropdown-menu/index.js"
import { Separator } from "$lib/components/ui/separator/index.js"
import { toasts } from "$lib/stores/toasts.js"
import { onMount } from "svelte"

let activeSessionId: string | undefined = $state()
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

function handleExit() {
  if (activeSessionId) {
    removeTerminal(activeSessionId)
    activeSessionId = $terminals.length > 0 ? $terminals[0].id : undefined
  }
}
</script>

<div class="flex min-h-0 flex-1 flex-col">
  <!-- Tab bar -->
  <div class="flex items-center gap-1 border-b bg-muted/30 px-2">
    <!-- Tabs -->
    <div class="flex min-w-0 flex-1 items-center gap-0.5 overflow-x-auto py-1.5">
      {#each $terminals as session (session.id)}
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div
          class="group/tab flex max-w-48 cursor-pointer items-center gap-1.5 rounded-md px-3 py-1.5 text-sm transition-colors {activeSessionId === session.id
            ? 'bg-background text-foreground shadow-sm border'
            : 'text-muted-foreground hover:bg-background/60 hover:text-foreground'}"
          role="tab"
          tabindex="0"
          aria-selected={activeSessionId === session.id}
          onclick={() => (activeSessionId = session.id)}
          ondblclick={() => startRename(session.id, session.label)}
          onkeydown={(e) => { if (e.key === "Enter" || e.key === " ") activeSessionId = session.id }}
        >
          {#if session.type === "ssh"}
            <Monitor class="h-3.5 w-3.5 shrink-0" />
          {:else}
            <TerminalIcon class="h-3.5 w-3.5 shrink-0" />
          {/if}

          {#if renamingId === session.id}
            <!-- svelte-ignore a11y_autofocus -->
            <input
              class="w-20 rounded border bg-background px-1 text-xs"
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
            <span class="truncate">{session.label}</span>
          {/if}

          <button
            type="button"
            class="ml-auto shrink-0 rounded p-0.5 opacity-0 transition-opacity hover:bg-muted group-hover/tab:opacity-60 hover:!opacity-100"
            title="Close"
            onclick={(e) => { e.stopPropagation(); closeSession(session.id) }}
          >
            <X class="h-3 w-3" />
          </button>
        </div>
      {/each}
    </div>

    <Separator orientation="vertical" class="mx-1 h-5" />

    <!-- Actions -->
    <div class="flex shrink-0 items-center gap-0.5">
      <Button variant="ghost" size="icon-sm" onclick={createShell} title="New shell">
        <Plus class="h-4 w-4" />
      </Button>

      {#if runningWorkspaces.length > 0}
        <DropdownMenu.Root>
          <DropdownMenu.Trigger>
            {#snippet child({ props })}
              <Button variant="ghost" size="icon-sm" {...props} title="SSH into workspace">
                <Monitor class="h-4 w-4" />
              </Button>
            {/snippet}
          </DropdownMenu.Trigger>
          <DropdownMenu.Content align="end" class="w-56">
            <DropdownMenu.Label>Running Workspaces</DropdownMenu.Label>
            <DropdownMenu.Separator />
            {#each runningWorkspaces as ws (ws.id)}
              <DropdownMenu.Item onclick={() => createSsh(ws.id)}>
                <Monitor class="mr-2 h-4 w-4" />
                {ws.id}
              </DropdownMenu.Item>
            {/each}
          </DropdownMenu.Content>
        </DropdownMenu.Root>
      {/if}
    </div>
  </div>

  <!-- Terminal content -->
  {#if $terminals.length > 0}
    <div class="relative min-h-0 flex-1">
      {#each $terminals as session (session.id)}
        <div class="absolute inset-0" class:hidden={session.id !== activeSessionId}>
          <TerminalComponent sessionId={session.id} active={session.id === activeSessionId} onExit={handleExit} />
        </div>
      {/each}
    </div>
  {:else}
    <div class="flex flex-1 items-center justify-center">
      <div class="flex flex-col items-center gap-4 text-center">
        <div class="rounded-full bg-muted p-4">
          <SquareTerminal class="h-8 w-8 text-muted-foreground" />
        </div>
        <div>
          <p class="text-lg font-medium">No active terminals</p>
          <p class="mt-1 text-sm text-muted-foreground">Create a local shell or SSH into a running workspace</p>
        </div>
        <div class="flex gap-2">
          <Button onclick={createShell}>
            <Plus class="mr-2 h-4 w-4" />
            New Shell
          </Button>
          {#if runningWorkspaces.length > 0}
            <DropdownMenu.Root>
              <DropdownMenu.Trigger>
                {#snippet child({ props })}
                  <Button variant="outline" {...props}>
                    <Monitor class="mr-2 h-4 w-4" />
                    SSH into Workspace
                  </Button>
                {/snippet}
              </DropdownMenu.Trigger>
              <DropdownMenu.Content class="w-56">
                <DropdownMenu.Label>Running Workspaces</DropdownMenu.Label>
                <DropdownMenu.Separator />
                {#each runningWorkspaces as ws (ws.id)}
                  <DropdownMenu.Item onclick={() => createSsh(ws.id)}>
                    <Monitor class="mr-2 h-4 w-4" />
                    {ws.id}
                  </DropdownMenu.Item>
                {/each}
              </DropdownMenu.Content>
            </DropdownMenu.Root>
          {/if}
        </div>
      </div>
    </div>
  {/if}
</div>
