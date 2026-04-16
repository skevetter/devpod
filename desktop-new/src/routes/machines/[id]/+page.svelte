<script lang="ts">
import { page } from "$app/stores"
import { goto } from "$app/navigation"
import { onMount, onDestroy } from "svelte"
import { Button } from "$lib/components/ui/button/index.js"
import { Separator } from "$lib/components/ui/separator/index.js"
import { badgeVariants } from "$lib/components/ui/badge/index.js"
import * as Tabs from "$lib/components/ui/tabs/index.js"
import { ScrollArea } from "$lib/components/ui/scroll-area/index.js"
import ConfirmDialog from "$lib/components/layout/ConfirmDialog.svelte"
import { machines } from "$lib/stores/machines.js"
import {
  machineStart,
  machineStop,
  machineDelete,
  machineStatus,
  auditByResource,
} from "$lib/ipc/commands.js"
import { toasts } from "$lib/stores/toasts.js"
import type { AuditEntry } from "$lib/types/index.js"
import { formatTimestamp } from "$lib/utils/time.js"

let id = $derived($page.params.id as string)
let machine = $derived($machines.find((m) => m.id === id))

let status = $state<string | null>(null)
let polling = $state(false)
let pollTimer: ReturnType<typeof setInterval> | null = null

let isRunning = $derived(
  status?.toLowerCase() === "running" ||
    machine?.status?.toLowerCase() === "running",
)
let isStopped = $derived.by(() => {
  const s = (status ?? machine?.status ?? "").toLowerCase()
  return !s || s === "stopped" || s === "notfound"
})

function statusBadgeVariant(): "default" | "secondary" | "outline" {
  if (isRunning) return "default"
  return "outline"
}

let auditEntries = $state<AuditEntry[]>([])
let auditLoading = $state(false)
let confirmDeleteOpen = $state(false)
let confirmForceDeleteOpen = $state(false)
let deleting = $state(false)

onMount(async () => {
  await refreshStatus()
  loadAudit()

  // Poll status every 5 seconds
  pollTimer = setInterval(refreshStatus, 5000)
})

onDestroy(() => {
  if (pollTimer) clearInterval(pollTimer)
})

async function refreshStatus() {
  try {
    status = await machineStatus(id)
  } catch {
    // Status fetch failed
  }
}

async function loadAudit() {
  auditLoading = true
  try {
    auditEntries = await auditByResource("machine", id)
  } catch {
    auditEntries = []
  } finally {
    auditLoading = false
  }
}

async function handleStart() {
  polling = true
  try {
    await machineStart(id)
    toasts.success(`Started ${id}`)
    await refreshStatus()
  } catch (err) {
    toasts.error(`Failed to start: ${err}`)
  } finally {
    polling = false
  }
}

async function handleStop() {
  polling = true
  try {
    await machineStop(id)
    toasts.success(`Stopped ${id}`)
    await refreshStatus()
  } catch (err) {
    toasts.error(`Failed to stop: ${err}`)
  } finally {
    polling = false
  }
}

async function handleDelete(force = false) {
  deleting = true
  try {
    await machineDelete(id, force)
    toasts.success(`Deleted ${id}`)
    confirmDeleteOpen = false
    confirmForceDeleteOpen = false
    goto("/machines")
  } catch (err) {
    toasts.error(`Failed to delete: ${err}`)
  } finally {
    deleting = false
  }
}
</script>

<div class="space-y-6">
  <div class="flex items-center gap-4">
    <Button variant="ghost" size="sm" onclick={() => goto("/machines")}>
      &larr; Back
    </Button>
    <h1 class="text-2xl font-bold">{id}</h1>
    {#if status}
      <span class={badgeVariants({ variant: statusBadgeVariant() })}>{status}</span>
    {/if}
    {#if polling}
      <span class="text-xs text-muted-foreground animate-pulse">updating...</span>
    {/if}
  </div>

  <div class="flex gap-2">
    {#if isStopped}
      <Button size="sm" onclick={handleStart} disabled={polling}>
        {polling ? "Starting..." : "Start"}
      </Button>
    {/if}
    {#if isRunning}
      <Button variant="outline" size="sm" onclick={handleStop} disabled={polling}>
        {polling ? "Stopping..." : "Stop"}
      </Button>
    {/if}
    <Button variant="destructive" size="sm" onclick={() => (confirmDeleteOpen = true)} disabled={polling}>Delete</Button>
    <Button variant="outline" size="sm" onclick={() => (confirmForceDeleteOpen = true)} disabled={polling}>Force Delete</Button>
  </div>

  <Separator />

  {#if !machine}
    <p class="text-muted-foreground">Machine not found.</p>
  {:else}
    <Tabs.Root value="details">
      <Tabs.List>
        <Tabs.Trigger value="details">Details</Tabs.Trigger>
        <Tabs.Trigger value="activity">Activity</Tabs.Trigger>
      </Tabs.List>

      <Tabs.Content value="details">
        <div class="mt-4 grid grid-cols-2 gap-4 text-sm">
          <div class="text-muted-foreground">ID</div>
          <div>{machine.id}</div>

          <div class="text-muted-foreground">Provider</div>
          <div>{machine.provider?.name ?? "N/A"}</div>

          <div class="text-muted-foreground">Status</div>
          <div>{status ?? machine.status ?? "Unknown"}</div>

          <div class="text-muted-foreground">Created</div>
          <div>{machine.creationTimestamp ?? "N/A"}</div>

          <div class="text-muted-foreground">Last Used</div>
          <div>{machine.lastUsedTimestamp ?? "N/A"}</div>
        </div>
      </Tabs.Content>

      <Tabs.Content value="activity">
        <div class="mt-4 space-y-4">
          {#if auditLoading}
            <p class="text-sm text-muted-foreground">Loading activity...</p>
          {:else if auditEntries.length === 0}
            <p class="text-sm text-muted-foreground">
              No activity recorded for this machine.
            </p>
          {:else}
            <ScrollArea class="h-80 rounded-md border">
              <div class="divide-y">
                {#each auditEntries as entry}
                  <div class="flex items-center gap-3 px-4 py-3">
                    <span
                      class={badgeVariants({
                        variant: entry.success ? "default" : "destructive",
                      })}
                    >
                      {entry.action}
                    </span>
                    <div class="min-w-0 flex-1">
                      {#if entry.details}
                        <span class="text-sm text-muted-foreground">{entry.details}</span>
                      {/if}
                    </div>
                    <span class="shrink-0 text-xs text-muted-foreground">
                      {formatTimestamp(entry.timestamp)}
                    </span>
                  </div>
                {/each}
              </div>
            </ScrollArea>
          {/if}
        </div>
      </Tabs.Content>
    </Tabs.Root>
  {/if}
</div>

<ConfirmDialog
  bind:open={confirmDeleteOpen}
  title="Delete machine"
  description="This will permanently delete machine '{id}'. This action cannot be undone."
  confirmLabel="Delete"
  loading={deleting}
  onconfirm={() => handleDelete(false)}
/>

<ConfirmDialog
  bind:open={confirmForceDeleteOpen}
  title="Force delete machine"
  description="This will forcefully delete machine '{id}', skipping graceful shutdown. Use this only if the machine is stuck. This action cannot be undone."
  confirmLabel="Force Delete"
  loading={deleting}
  onconfirm={() => handleDelete(true)}
/>
