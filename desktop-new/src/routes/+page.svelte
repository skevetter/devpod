<script lang="ts">
import { onMount } from "svelte"
import { goto } from "$app/navigation"
import { Button } from "$lib/components/ui/button/index.js"
import { Separator } from "$lib/components/ui/separator/index.js"
import { badgeVariants } from "$lib/components/ui/badge/index.js"
import { ScrollArea } from "$lib/components/ui/scroll-area/index.js"
import { workspaces } from "$lib/stores/workspaces.js"
import { providers } from "$lib/stores/providers.js"
import { machines } from "$lib/stores/machines.js"
import { activeContext } from "$lib/stores/contexts.js"
import { auditRecent, workspaceStop } from "$lib/ipc/commands.js"
import { toasts } from "$lib/stores/toasts.js"
import { Box, Plug, Server } from "lucide-svelte"
import type { AuditEntry } from "$lib/types/index.js"
import { formatTimestamp } from "$lib/utils/time.js"

let activity = $state<AuditEntry[]>([])

onMount(async () => {
  try {
    activity = await auditRecent(10)
  } catch {
    activity = []
  }
})

let runningWorkspaces = $derived(
  $workspaces.filter((ws) => ws.status?.toLowerCase() === "running"),
)
let runningMachines = $derived(
  $machines.filter((m) => m.status?.toLowerCase() === "running"),
)

const stats = $derived([
  {
    label: "Workspaces",
    count: $workspaces.length,
    href: "/workspaces",
    icon: Box,
    sub:
      runningWorkspaces.length > 0
        ? `${runningWorkspaces.length} running`
        : undefined,
  },
  {
    label: "Providers",
    count: $providers.length,
    href: "/providers",
    icon: Plug,
    sub: undefined as string | undefined,
  },
  {
    label: "Machines",
    count: $machines.length,
    href: "/machines",
    icon: Server,
    sub:
      runningMachines.length > 0
        ? `${runningMachines.length} running`
        : undefined,
  },
])

function resourceHref(entry: AuditEntry): string | null {
  if (!entry.resourceId) return null
  switch (entry.resourceType) {
    case "workspace":
      return `/workspaces/${entry.resourceId}`
    case "provider":
      return `/providers/${entry.resourceId}`
    case "machine":
      return `/machines/${entry.resourceId}`
    default:
      return null
  }
}

async function quickStop(wsId: string) {
  try {
    await workspaceStop(wsId)
    toasts.success(`Stopping ${wsId}...`)
  } catch (err) {
    toasts.error(`Failed to stop: ${err}`)
  }
}
</script>

<div class="space-y-6">
  <div>
    <h1 class="text-2xl font-bold">Dashboard</h1>
    {#if $activeContext}
      <p class="mt-1 text-sm text-muted-foreground">
        Context: <span class="font-medium">{$activeContext}</span>
      </p>
    {/if}
  </div>

  <div class="grid gap-4 sm:grid-cols-3">
    {#each stats as stat (stat.label)}
      {@const Icon = stat.icon}
      <button
        type="button"
        class="rounded-lg border bg-card p-6 text-left text-card-foreground shadow-sm transition-colors hover:bg-accent/50"
        onclick={() => goto(stat.href)}
      >
        <div class="flex items-center justify-between">
          <div class="text-3xl font-bold">{stat.count}</div>
          <Icon class="h-5 w-5 text-muted-foreground" />
        </div>
        <div class="mt-1 text-sm text-muted-foreground">{stat.label}</div>
        {#if stat.sub}
          <div class="mt-1 text-xs text-green-600 dark:text-green-400">{stat.sub}</div>
        {/if}
      </button>
    {/each}
  </div>

  <div class="flex gap-2">
    <Button onclick={() => goto("/workspaces/new")}>New Workspace</Button>
    <Button variant="outline" onclick={() => goto("/providers/add")}>Add Provider</Button>
  </div>

  {#if runningWorkspaces.length > 0}
    <div class="space-y-3">
      <h2 class="text-lg font-semibold">Active Workspaces</h2>
      <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {#each runningWorkspaces as ws (ws.id)}
          <div class="rounded-lg border bg-card p-4 shadow-sm">
            <div class="flex items-start justify-between gap-2">
              <button
                class="min-w-0 truncate font-medium hover:underline text-left"
                onclick={() => goto(`/workspaces/${ws.id}`)}
              >
                {ws.id}
              </button>
              <div class="flex shrink-0 gap-1">
                <Button variant="outline" size="sm" onclick={() => goto(`/workspaces/${ws.id}`)}>
                  Open
                </Button>
                <Button variant="ghost" size="sm" onclick={() => quickStop(ws.id)}>
                  Stop
                </Button>
              </div>
            </div>
            <div class="flex items-center gap-2 mt-1">
              {#if ws.provider?.name}
                <span class="text-xs text-muted-foreground">{ws.provider.name}</span>
              {/if}
              <span class={badgeVariants({ variant: "default" })}>{ws.status}</span>
            </div>
          </div>
        {/each}
      </div>
    </div>
  {/if}

  <Separator />

  <div class="space-y-4">
    <h2 class="text-lg font-semibold">Recent Activity</h2>

    {#if activity.length === 0}
      <p class="text-sm text-muted-foreground">No recent activity.</p>
    {:else}
      <ScrollArea class="h-64 rounded-md border">
        <div class="divide-y">
          {#each activity as entry}
            {@const href = resourceHref(entry)}
            <div class="flex items-center gap-3 px-4 py-3">
              <span
                class={badgeVariants({
                  variant: entry.success ? "default" : "destructive",
                })}
              >
                {entry.action}
              </span>
              <div class="min-w-0 flex-1">
                <span class="text-sm">
                  {entry.resourceType}
                  {#if entry.resourceId}
                    {#if href}
                      <a class="font-medium hover:underline" {href}>{entry.resourceId}</a>
                    {:else}
                      <span class="font-medium">{entry.resourceId}</span>
                    {/if}
                  {/if}
                </span>
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
</div>
