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
import { auditRecent } from "$lib/ipc/commands.js"
import type { AuditEntry } from "$lib/types/index.js"

let activity = $state<AuditEntry[]>([])

onMount(async () => {
  try {
    activity = await auditRecent(10)
  } catch {
    activity = []
  }
})

function formatTimestamp(ts: string): string {
  try {
    return new Date(ts).toLocaleString()
  } catch {
    return ts
  }
}

const stats = $derived([
  { label: "Workspaces", count: $workspaces.length, href: "/workspaces" },
  { label: "Providers", count: $providers.length, href: "/providers" },
  { label: "Machines", count: $machines.length, href: "/machines" },
])
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
      <button
        type="button"
        class="rounded-lg border bg-card p-6 text-left text-card-foreground shadow-sm transition-colors hover:bg-accent/50"
        onclick={() => goto(stat.href)}
      >
        <div class="text-3xl font-bold">{stat.count}</div>
        <div class="mt-1 text-sm text-muted-foreground">{stat.label}</div>
      </button>
    {/each}
  </div>

  <div class="flex gap-2">
    <Button onclick={() => goto("/workspaces/new")}>New Workspace</Button>
    <Button variant="outline" onclick={() => goto("/providers/add")}>Add Provider</Button>
  </div>

  <Separator />

  <div class="space-y-4">
    <h2 class="text-lg font-semibold">Recent Activity</h2>

    {#if activity.length === 0}
      <p class="text-sm text-muted-foreground">No recent activity.</p>
    {:else}
      <ScrollArea class="h-64 rounded-md border">
        <div class="divide-y">
          {#each activity as entry}
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
                    <span class="font-medium">{entry.resourceId}</span>
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
