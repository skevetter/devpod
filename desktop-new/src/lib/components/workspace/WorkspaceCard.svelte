<script lang="ts">
import { goto } from "$app/navigation"
import { Button } from "$lib/components/ui/button/index.js"
import { badgeVariants } from "$lib/components/ui/badge/index.js"
import ConfirmDialog from "$lib/components/layout/ConfirmDialog.svelte"
import {
  workspaceUp,
  workspaceStop,
  workspaceDelete,
} from "$lib/ipc/commands.js"
import { toasts } from "$lib/stores/toasts.js"
import type { Workspace } from "$lib/types/index.js"
import { timeAgo } from "$lib/utils/time.js"

let { workspace }: { workspace: Workspace } = $props()
let confirmDeleteOpen = $state(false)
let deleting = $state(false)
let acting = $state(false)

let isRunning = $derived(workspace.status?.toLowerCase() === "running")
let isStopped = $derived(
  !workspace.status ||
    workspace.status.toLowerCase() === "stopped" ||
    workspace.status.toLowerCase() === "notfound",
)
let isBusy = $derived(workspace.status?.toLowerCase() === "busy")

function sourceDisplay(ws: Workspace): string {
  if (ws.source?.gitRepository) return ws.source.gitRepository
  if (ws.source?.localFolder) return ws.source.localFolder
  if (ws.source?.image) return ws.source.image
  return "Unknown source"
}

async function handleStart(e: Event) {
  e.stopPropagation()
  acting = true
  try {
    await workspaceUp({ source: workspace.id })
    toasts.success(`Starting ${workspace.id}`)
  } catch (err) {
    toasts.error(`Failed to start: ${err}`)
  } finally {
    acting = false
  }
}

async function handleStop(e: Event) {
  e.stopPropagation()
  acting = true
  try {
    await workspaceStop(workspace.id)
    toasts.success(`Stopped ${workspace.id}`)
  } catch (err) {
    toasts.error(`Failed to stop: ${err}`)
  } finally {
    acting = false
  }
}

function openDeleteConfirm(e: Event) {
  e.stopPropagation()
  confirmDeleteOpen = true
}

async function handleDelete() {
  deleting = true
  try {
    await workspaceDelete(workspace.id)
    toasts.success(`Deleted ${workspace.id}`)
    confirmDeleteOpen = false
  } catch (err) {
    toasts.error(`Failed to delete: ${err}`)
  } finally {
    deleting = false
  }
}
</script>

<button
  type="button"
  class="rounded-lg border bg-card p-4 text-left text-card-foreground shadow-sm transition-colors hover:bg-accent/50 w-full"
  onclick={() => goto(`/workspaces/${workspace.id}`)}
>
  <div class="flex items-start justify-between gap-2">
    <h3 class="font-semibold truncate">{workspace.id}</h3>
    <span class="text-xs text-muted-foreground whitespace-nowrap">
      {timeAgo(workspace.lastUsedTimestamp)}
    </span>
  </div>

  <p class="mt-1 text-sm text-muted-foreground truncate">
    {sourceDisplay(workspace)}
  </p>

  <div class="mt-3 flex flex-wrap items-center gap-2">
    {#if workspace.provider?.name}
      <span class={badgeVariants({ variant: "secondary" })}>
        {workspace.provider.name}
      </span>
    {/if}
    {#if workspace.ide?.name}
      <span class={badgeVariants({ variant: "outline" })}>
        {workspace.ide.name}
      </span>
    {/if}
    {#if workspace.status}
      <span
        class={badgeVariants({
          variant: isRunning ? "default" : isBusy ? "secondary" : "outline",
        })}
      >
        {workspace.status}
      </span>
    {/if}
  </div>

  <div class="mt-3 flex gap-2">
    {#if isStopped}
      <Button size="sm" onclick={handleStart} disabled={acting}>
        {acting ? "Starting..." : "Start"}
      </Button>
    {/if}
    {#if isRunning || isBusy}
      <Button variant="outline" size="sm" onclick={handleStop} disabled={acting}>
        {acting ? "Stopping..." : "Stop"}
      </Button>
    {/if}
    <Button variant="destructive" size="sm" onclick={openDeleteConfirm} disabled={acting}>Delete</Button>
  </div>
</button>

<ConfirmDialog
  bind:open={confirmDeleteOpen}
  title="Delete workspace"
  description="This will permanently delete workspace '{workspace.id}' and all associated data. This action cannot be undone."
  confirmLabel="Delete"
  loading={deleting}
  onconfirm={handleDelete}
/>
