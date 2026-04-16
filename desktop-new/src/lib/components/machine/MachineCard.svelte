<script lang="ts">
import { goto } from "$app/navigation"
import { Button } from "$lib/components/ui/button/index.js"
import { badgeVariants } from "$lib/components/ui/badge/index.js"
import ConfirmDialog from "$lib/components/layout/ConfirmDialog.svelte"
import { machineStart, machineStop, machineDelete } from "$lib/ipc/commands.js"
import { toasts } from "$lib/stores/toasts.js"
import type { Machine } from "$lib/types/index.js"
import { timeAgo } from "$lib/utils/time.js"

let { machine }: { machine: Machine } = $props()
let confirmDeleteOpen = $state(false)
let deleting = $state(false)
let acting = $state(false)

let isRunning = $derived(machine.status?.toLowerCase() === "running")
let isStopped = $derived(
  !machine.status ||
    machine.status.toLowerCase() === "stopped" ||
    machine.status.toLowerCase() === "notfound",
)

async function handleStart(e: Event) {
  e.stopPropagation()
  acting = true
  try {
    await machineStart(machine.id)
    toasts.success(`Started ${machine.id}`)
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
    await machineStop(machine.id)
    toasts.success(`Stopped ${machine.id}`)
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
    await machineDelete(machine.id)
    toasts.success(`Deleted ${machine.id}`)
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
  onclick={() => goto(`/machines/${machine.id}`)}
>
  <div class="flex items-start justify-between gap-2">
    <h3 class="font-semibold truncate">{machine.id}</h3>
    {#if machine.status}
      <span class={badgeVariants({ variant: isRunning ? "default" : "outline" })}>{machine.status}</span>
    {/if}
  </div>

  <div class="mt-2 flex flex-wrap items-center gap-2">
    {#if machine.provider?.name}
      <span class={badgeVariants({ variant: "secondary" })}>{machine.provider.name}</span>
    {/if}
    <span class="text-xs text-muted-foreground">
      Created {timeAgo(machine.creationTimestamp)}
    </span>
  </div>

  <div class="mt-3 flex gap-2">
    {#if isStopped}
      <Button size="sm" onclick={handleStart} disabled={acting}>
        {acting ? "Starting..." : "Start"}
      </Button>
    {/if}
    {#if isRunning}
      <Button variant="outline" size="sm" onclick={handleStop} disabled={acting}>
        {acting ? "Stopping..." : "Stop"}
      </Button>
    {/if}
    <Button variant="destructive" size="sm" onclick={openDeleteConfirm} disabled={acting}>Delete</Button>
  </div>
</button>

<ConfirmDialog
  bind:open={confirmDeleteOpen}
  title="Delete machine"
  description="This will permanently delete machine '{machine.id}'. This action cannot be undone."
  confirmLabel="Delete"
  loading={deleting}
  onconfirm={handleDelete}
/>
