<script lang="ts">
import { Input } from "$lib/components/ui/input/index.js"
import MachineCard from "$lib/components/machine/MachineCard.svelte"
import { machines } from "$lib/stores/machines.js"

let search = $state("")
let sortBy = $state<"name" | "created">("name")

let filtered = $derived.by(() => {
  const q = search.toLowerCase()
  let list = $machines.filter((m) => {
    if (!q) return true
    return (
      m.id.toLowerCase().includes(q) ||
      (m.provider?.name ?? "").toLowerCase().includes(q) ||
      (m.status ?? "").toLowerCase().includes(q)
    )
  })

  if (sortBy === "created") {
    list = [...list].sort((a, b) =>
      (b.creationTimestamp ?? "").localeCompare(a.creationTimestamp ?? ""),
    )
  }

  return list
})
</script>

<div class="space-y-6">
  <div class="flex items-center justify-between">
    <h1 class="text-2xl font-bold">Machines</h1>
  </div>

  <div class="flex gap-2">
    <Input
      placeholder="Search by name, provider, status..."
      value={search}
      oninput={(e) => (search = e.currentTarget.value)}
      class="flex-1"
    />
    <select
      class="h-10 rounded-md border border-input bg-background px-3 text-sm"
      value={sortBy}
      onchange={(e) => (sortBy = e.currentTarget.value as "name" | "created")}
    >
      <option value="name">Name</option>
      <option value="created">Newest</option>
    </select>
  </div>

  {#if filtered.length === 0}
    <div class="flex flex-col items-center justify-center gap-4 py-16 text-center">
      <p class="text-muted-foreground">
        {search ? "No machines match your search." : "No machines yet."}
      </p>
      <p class="text-sm text-muted-foreground">
        Machines are created automatically when you start a workspace with a provider that supports them.
      </p>
    </div>
  {:else}
    <div class="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
      {#each filtered as machine (machine.id)}
        <MachineCard {machine} />
      {/each}
    </div>
  {/if}
</div>
