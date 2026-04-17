<script lang="ts">
import { Plug, SearchX } from "@lucide/svelte"
import { goto } from "$app/navigation"
import { page } from "$app/stores"
import { Button } from "$lib/components/ui/button/index.js"
import { Input } from "$lib/components/ui/input/index.js"
import * as Select from "$lib/components/ui/select/index.js"
import CardSkeleton from "$lib/components/ui/skeleton/CardSkeleton.svelte"
import ProviderCard from "$lib/components/provider/ProviderCard.svelte"
import ProviderSheet from "$lib/components/provider/ProviderSheet.svelte"
import { providers, providersLoading } from "$lib/stores/providers.js"

let search = $state("")
let sortBy = $state<"name" | "version">("name")

// Sheet state for setup flow (redirected from /providers/add)
let setupProviderName = $state<string | null>(null)
let setupSheetOpen = $state(false)

let setupProvider = $derived(
  setupProviderName
    ? $providers.find((p) => p.name === setupProviderName)
    : undefined,
)

// Auto-open sheet when arriving with ?setup=<name>
$effect(() => {
  const setup = $page.url.searchParams.get("setup")
  if (setup && $providers.length > 0) {
    setupProviderName = setup
    setupSheetOpen = true
    // Clean up the URL
    const url = new URL($page.url)
    url.searchParams.delete("setup")
    history.replaceState({}, "", url.pathname)
  }
})

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
    <Select.Root type="single" bind:value={sortBy}>
      <Select.Trigger class="w-32">
        <span>{sortBy === "name" ? "Name" : "Version"}</span>
      </Select.Trigger>
      <Select.Content>
        <Select.Item value="name" label="Name" />
        <Select.Item value="version" label="Version" />
      </Select.Content>
    </Select.Root>
  </div>

  {#if $providersLoading}
    <div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
      {#each Array(3) as _}
        <CardSkeleton />
      {/each}
    </div>
  {:else if filtered.length === 0}
    <div class="flex flex-col items-center justify-center gap-4 py-16 text-center">
      {#if search}
        <SearchX class="h-10 w-10 text-muted-foreground" />
        <p class="text-muted-foreground">No providers match your search.</p>
      {:else}
        <Plug class="h-10 w-10 text-muted-foreground" />
        <p class="text-muted-foreground">No providers configured yet.</p>
        <Button onclick={() => goto("/providers/add")}>Add your first provider</Button>
      {/if}
    </div>
  {:else}
    <div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
      {#each filtered as provider (provider.name)}
        <ProviderCard {provider} />
      {/each}
    </div>
  {/if}
</div>

{#if setupProvider}
  <ProviderSheet
    provider={setupProvider}
    bind:open={setupSheetOpen}
    setup={true}
    ondeleted={() => { setupProviderName = null }}
  />
{/if}
