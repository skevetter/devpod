<script lang="ts">
import { goto } from "$app/navigation"
import { Button } from "$lib/components/ui/button/index.js"
import { Input } from "$lib/components/ui/input/index.js"
import ProviderCard from "$lib/components/provider/ProviderCard.svelte"
import { providers } from "$lib/stores/providers.js"

let search = $state("")
let sortBy = $state<"name" | "version">("name")

let filtered = $derived.by(() => {
  const q = search.toLowerCase()
  let list = $providers.filter((p) => {
    if (!q) return true
    return (
      p.name.toLowerCase().includes(q) ||
      (p.description ?? "").toLowerCase().includes(q) ||
      (p.version ?? "").toLowerCase().includes(q)
    )
  })

  if (sortBy === "version") {
    list = [...list].sort((a, b) =>
      (b.version ?? "").localeCompare(a.version ?? ""),
    )
  }

  return list
})
</script>

<div class="space-y-6">
  <div class="flex items-center justify-between">
    <h1 class="text-2xl font-bold">Providers</h1>
    <Button onclick={() => goto("/providers/add")}>Add Provider</Button>
  </div>

  <div class="flex gap-2">
    <Input
      placeholder="Search by name, description, version..."
      value={search}
      oninput={(e) => (search = e.currentTarget.value)}
      class="flex-1"
    />
    <select
      class="h-10 rounded-md border border-input bg-background px-3 text-sm"
      value={sortBy}
      onchange={(e) => (sortBy = e.currentTarget.value as "name" | "version")}
    >
      <option value="name">Name</option>
      <option value="version">Version</option>
    </select>
  </div>

  {#if filtered.length === 0}
    <div class="flex flex-col items-center justify-center gap-4 py-16 text-center">
      <p class="text-muted-foreground">
        {search ? "No providers match your search." : "No providers configured yet."}
      </p>
      {#if !search}
        <Button onclick={() => goto("/providers/add")}>Add your first provider</Button>
      {/if}
    </div>
  {:else}
    <div class="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
      {#each filtered as provider (provider.name)}
        <ProviderCard {provider} />
      {/each}
    </div>
  {/if}
</div>
