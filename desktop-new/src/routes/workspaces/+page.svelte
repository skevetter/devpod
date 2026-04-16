<script lang="ts">
import { goto } from "$app/navigation"
import { Button } from "$lib/components/ui/button/index.js"
import { Input } from "$lib/components/ui/input/index.js"
import WorkspaceCard from "$lib/components/workspace/WorkspaceCard.svelte"
import { workspaces, workspacesLoading } from "$lib/stores/workspaces.js"
import { Box, SearchX } from "lucide-svelte"

let search = $state("")
let sortBy = $state<"recent" | "name">("recent")

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
  // "recent" is already the default sort from the store

  return list
})
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
    <select
      class="h-10 rounded-md border border-input bg-background px-3 text-sm"
      value={sortBy}
      onchange={(e) => (sortBy = e.currentTarget.value as "recent" | "name")}
    >
      <option value="recent">Recent</option>
      <option value="name">Name</option>
    </select>
  </div>

  {#if $workspacesLoading}
    <div class="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
      {#each Array(3) as _}
        <div class="h-36 animate-pulse rounded-lg border bg-muted"></div>
      {/each}
    </div>
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
    <div class="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
      {#each filtered as workspace (workspace.id)}
        <WorkspaceCard {workspace} />
      {/each}
    </div>
  {/if}
</div>
