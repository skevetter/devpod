<script lang="ts">
import {
  ArrowDownAZ,
  Box,
  Clock,
  ChevronsUpDown,
  Ellipsis,
  Play,
  SearchX,
  Square,
  Trash2,
} from "@lucide/svelte"
import { goto } from "$app/navigation"
import { Button } from "$lib/components/ui/button/index.js"
import { badgeVariants } from "$lib/components/ui/badge/index.js"
import * as DropdownMenu from "$lib/components/ui/dropdown-menu/index.js"
import { Input } from "$lib/components/ui/input/index.js"
import * as Table from "$lib/components/ui/table/index.js"
import TableSkeleton from "$lib/components/ui/skeleton/TableSkeleton.svelte"
import ConfirmDialog from "$lib/components/layout/ConfirmDialog.svelte"
import {
  workspaceUp,
  workspaceStop,
  workspaceDelete,
} from "$lib/ipc/commands.js"
import { toasts } from "$lib/stores/toasts.js"
import { workspaces, workspacesLoading } from "$lib/stores/workspaces.js"
import type { Workspace } from "$lib/types/index.js"
import { timeAgo } from "$lib/utils/time.js"

let search = $state("")
let sortBy = $state<"recent" | "name">("recent")

let actingOn = $state<string | null>(null)
let confirmDeleteId = $state<string | null>(null)
let confirmDeleteOpen = $state(false)
let deleting = $state(false)

let filtered = $derived.by(() => {
  const q = search.toLowerCase()
  let list = $workspaces.filter((ws) => {
    if (!q) return true
    return (
      ws.id.toLowerCase().includes(q) ||
      (ws.source?.gitRepository ?? "").toLowerCase().includes(q) ||
      (ws.source?.localFolder ?? "").toLowerCase().includes(q) ||
      (ws.source?.image ?? "").toLowerCase().includes(q) ||
      (ws.provider?.name ?? "").toLowerCase().includes(q) ||
      (ws.ide?.name ?? "").toLowerCase().includes(q)
    )
  })

  if (sortBy === "name") {
    list = [...list].sort((a, b) => a.id.localeCompare(b.id))
  }

  return list
})

function sourceDisplay(ws: Workspace): string {
  if (ws.source?.gitRepository) return ws.source.gitRepository
  if (ws.source?.localFolder) return ws.source.localFolder
  if (ws.source?.image) return ws.source.image
  return ""
}

function statusVariant(status?: string): "default" | "secondary" | "outline" {
  const s = status?.toLowerCase()
  if (s === "running") return "default"
  if (s === "busy") return "secondary"
  return "outline"
}

function isRunning(ws: Workspace) {
  return ws.status?.toLowerCase() === "running"
}

function isStopped(ws: Workspace) {
  return (
    !ws.status ||
    ws.status.toLowerCase() === "stopped" ||
    ws.status.toLowerCase() === "notfound"
  )
}

async function handleStart(ws: Workspace) {
  actingOn = ws.id
  try {
    await workspaceUp({ source: ws.id })
    toasts.success(`Starting ${ws.id}...`)
  } catch (err) {
    toasts.error(`Failed to start: ${err}`)
  } finally {
    actingOn = null
  }
}

async function handleStop(ws: Workspace) {
  actingOn = ws.id
  try {
    await workspaceStop(ws.id)
    toasts.success(`Stopping ${ws.id}...`)
  } catch (err) {
    toasts.error(`Failed to stop: ${err}`)
  } finally {
    actingOn = null
  }
}

async function handleDelete() {
  if (!confirmDeleteId) return
  deleting = true
  try {
    await workspaceDelete(confirmDeleteId)
    toasts.success(`Deleted ${confirmDeleteId}`)
    confirmDeleteOpen = false
    confirmDeleteId = null
  } catch (err) {
    toasts.error(`Failed to delete: ${err}`)
  } finally {
    deleting = false
  }
}
</script>

<div class="space-y-6">
  <div class="flex items-center justify-between">
    <h1 class="text-2xl font-bold">Workspaces</h1>
    <Button onclick={() => goto("/workspaces/new")}>Create Workspace</Button>
  </div>

  <div class="flex gap-2">
    <Input
      placeholder="Search by name, source, provider, IDE..."
      value={search}
      oninput={(e) => (search = e.currentTarget.value)}
      class="flex-1"
    />
    <DropdownMenu.Root>
      <DropdownMenu.Trigger>
        {#snippet child({ props })}
          <Button variant="outline" class="w-36 justify-between" {...props}>
            {#if sortBy === "recent"}
              <Clock class="mr-2 h-4 w-4" /> Recent
            {:else}
              <ArrowDownAZ class="mr-2 h-4 w-4" /> Name
            {/if}
            <ChevronsUpDown class="ml-auto h-4 w-4 opacity-50" />
          </Button>
        {/snippet}
      </DropdownMenu.Trigger>
      <DropdownMenu.Content align="end">
        <DropdownMenu.RadioGroup bind:value={sortBy}>
          <DropdownMenu.RadioItem value="recent">
            <Clock class="mr-2 h-4 w-4" /> Recent
          </DropdownMenu.RadioItem>
          <DropdownMenu.RadioItem value="name">
            <ArrowDownAZ class="mr-2 h-4 w-4" /> Name
          </DropdownMenu.RadioItem>
        </DropdownMenu.RadioGroup>
      </DropdownMenu.Content>
    </DropdownMenu.Root>
  </div>

  {#if $workspacesLoading}
    <TableSkeleton rows={5} columns={7} />
  {:else if filtered.length === 0}
    <div class="flex flex-col items-center justify-center gap-4 py-16 text-center">
      {#if search}
        <SearchX class="h-10 w-10 text-muted-foreground" />
        <p class="text-muted-foreground">No workspaces match your search.</p>
      {:else}
        <Box class="h-10 w-10 text-muted-foreground" />
        <p class="text-muted-foreground">No workspaces yet.</p>
        <Button onclick={() => goto("/workspaces/new")}>Create your first workspace</Button>
      {/if}
    </div>
  {:else}
    <div class="rounded-md border">
      <Table.Root>
        <Table.Header>
          <Table.Row>
            <Table.Head>Name</Table.Head>
            <Table.Head>Source</Table.Head>
            <Table.Head>Provider</Table.Head>
            <Table.Head>IDE</Table.Head>
            <Table.Head>Status</Table.Head>
            <Table.Head>Last Used</Table.Head>
            <Table.Head class="w-12"></Table.Head>
          </Table.Row>
        </Table.Header>
        <Table.Body>
          {#each filtered as ws (ws.id)}
            {@const busy = actingOn === ws.id}
            <Table.Row
              class="cursor-pointer"
              onclick={() => goto(`/workspaces/${ws.id}`)}
            >
              <Table.Cell class="font-medium">{ws.id}</Table.Cell>
              <Table.Cell class="max-w-[200px] truncate text-muted-foreground">{sourceDisplay(ws)}</Table.Cell>
              <Table.Cell>
                {#if ws.provider?.name}
                  <span class={badgeVariants({ variant: "secondary" })}>{ws.provider.name}</span>
                {/if}
              </Table.Cell>
              <Table.Cell>
                {#if ws.ide?.name}
                  <span class={badgeVariants({ variant: "outline" })}>{ws.ide.name}</span>
                {/if}
              </Table.Cell>
              <Table.Cell>
                {#if ws.status}
                  <span class={badgeVariants({ variant: statusVariant(ws.status) })}>{ws.status}</span>
                {/if}
              </Table.Cell>
              <Table.Cell class="text-sm text-muted-foreground">{timeAgo(ws.lastUsedTimestamp)}</Table.Cell>
              <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
              <Table.Cell onclick={(e: MouseEvent) => e.stopPropagation()}>
                <DropdownMenu.Root>
                  <DropdownMenu.Trigger>
                    {#snippet child({ props })}
                      <Button {...props} variant="ghost" size="icon" class="h-8 w-8">
                        <Ellipsis class="h-4 w-4" />
                        <span class="sr-only">Actions</span>
                      </Button>
                    {/snippet}
                  </DropdownMenu.Trigger>
                  <DropdownMenu.Content align="end">
                    {#if isRunning(ws)}
                      <DropdownMenu.Item onclick={() => goto(`/workspaces/${ws.id}?action=open-ide`)}>
                        <Play class="mr-2 h-4 w-4" />
                        Open IDE
                      </DropdownMenu.Item>
                      <DropdownMenu.Item onclick={() => handleStop(ws)} disabled={busy}>
                        <Square class="mr-2 h-4 w-4" />
                        Stop
                      </DropdownMenu.Item>
                    {:else if isStopped(ws)}
                      <DropdownMenu.Item onclick={() => handleStart(ws)} disabled={busy}>
                        <Play class="mr-2 h-4 w-4" />
                        Start
                      </DropdownMenu.Item>
                    {/if}
                    <DropdownMenu.Separator />
                    <DropdownMenu.Item
                      class="text-destructive data-[highlighted]:text-destructive"
                      onclick={() => { confirmDeleteId = ws.id; confirmDeleteOpen = true }}
                      disabled={busy}
                    >
                      <Trash2 class="mr-2 h-4 w-4" />
                      Delete
                    </DropdownMenu.Item>
                  </DropdownMenu.Content>
                </DropdownMenu.Root>
              </Table.Cell>
            </Table.Row>
          {/each}
        </Table.Body>
      </Table.Root>
    </div>
  {/if}
</div>

<ConfirmDialog
  bind:open={confirmDeleteOpen}
  title="Delete workspace"
  description="This will permanently delete workspace '{confirmDeleteId}' and all associated data. This action cannot be undone."
  confirmLabel="Delete"
  loading={deleting}
  onconfirm={handleDelete}
/>
