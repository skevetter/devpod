<script lang="ts">
import { goto } from "$app/navigation"
import { get } from "svelte/store"
import { Button } from "$lib/components/ui/button/index.js"
import { Input } from "$lib/components/ui/input/index.js"
import { Label } from "$lib/components/ui/label/index.js"
import ProviderIcon from "$lib/components/provider/ProviderIcon.svelte"
import { providerAdd } from "$lib/ipc/commands.js"
import { providers } from "$lib/stores/providers.js"
import { toasts } from "$lib/stores/toasts.js"

const PROVIDERS = [
  { name: "docker", description: "Local Docker containers" },
  { name: "ssh", description: "Remote SSH machines" },
  { name: "kubernetes", description: "Kubernetes clusters" },
  { name: "aws", description: "Amazon Web Services" },
  { name: "gcloud", description: "Google Cloud Platform" },
  { name: "azure", description: "Microsoft Azure" },
  { name: "digitalocean", description: "DigitalOcean Droplets" },
]

let providerSource = $state("")
let error = $state("")
let submitting = $state(false)

let pendingSource = $state<string | null>(null)
let pendingName = $state("")
let customName = $state("")

function nameExists(name: string): boolean {
  return get(providers).some((p) => p.name === name)
}

function handlePresetClick(source: string) {
  if (nameExists(source)) {
    pendingSource = source
    pendingName = ""
  } else {
    doAdd(source, source)
  }
}

function handleConfirmName() {
  const name = pendingName.trim()
  if (!name) {
    toasts.error("Name is required")
    return
  }
  if (nameExists(name)) {
    toasts.error(`Provider "${name}" already exists`)
    return
  }
  if (pendingSource) {
    doAdd(name, pendingSource)
    pendingSource = null
    pendingName = ""
  }
}

async function doAdd(name: string, source?: string) {
  submitting = true
  try {
    await providerAdd(name, source !== name ? source : undefined)
    toasts.success(`Added provider ${name}`)
    goto(`/providers?setup=${name}`)
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err)
    if (msg.includes("already exists")) {
      toasts.info(`Provider ${name} is already installed`)
      goto(`/providers?setup=${name}`)
      return
    }
    toasts.error(`Failed to add provider: ${msg}`)
  } finally {
    submitting = false
  }
}

async function handleSubmit() {
  const source = providerSource.trim()
  if (!source) {
    error = "Provider source is required"
    return
  }
  const name = customName.trim() || source
  if (nameExists(name)) {
    toasts.error(`Provider "${name}" already exists`)
    return
  }
  await doAdd(name, name !== source ? source : undefined)
}
</script>

<div class="mx-auto max-w-xl space-y-6">
  <div class="flex items-center gap-4">
    <Button variant="ghost" size="sm" onclick={() => goto("/providers")}>
      &larr; Back
    </Button>
    <h1 class="text-2xl font-bold">Add Provider</h1>
  </div>

  <form class="space-y-4" onsubmit={(e) => { e.preventDefault(); handleSubmit() }}>
    <div class="space-y-2">
      <Label>Provider Source</Label>
      <Input
        placeholder="e.g. docker, or github.com/org/provider"
        value={providerSource}
        oninput={(e) => (providerSource = e.currentTarget.value)}
      />
    </div>
    <div class="space-y-2">
      <Label>Name <span class="text-muted-foreground font-normal">(optional)</span></Label>
      <Input
        placeholder="Defaults to source name"
        value={customName}
        oninput={(e) => (customName = e.currentTarget.value)}
      />
    </div>

    <Button type="submit" disabled={submitting} class="w-full">
      {submitting ? "Adding..." : "Add Provider"}
    </Button>
  </form>

  {#if pendingSource}
    <div class="rounded-lg border bg-card p-4 space-y-3">
      <h3 class="text-sm font-semibold">
        Provider "{pendingSource}" already exists. Choose a different name:
      </h3>
      <form class="flex items-end gap-2" onsubmit={(e) => { e.preventDefault(); handleConfirmName() }}>
        <div class="flex-1 space-y-1">
          <Label>Provider Name</Label>
          <Input
            placeholder={`e.g. ${pendingSource}-2`}
            value={pendingName}
            oninput={(e) => (pendingName = e.currentTarget.value)}
          />
        </div>
        <Button type="submit" size="sm" disabled={submitting || !pendingName.trim()}>
          {submitting ? "Adding..." : "Add"}
        </Button>
        <Button variant="outline" size="sm" onclick={() => (pendingSource = null)}>
          Cancel
        </Button>
      </form>
    </div>
  {/if}

  <div class="space-y-3">
    <h2 class="text-lg font-semibold">Providers</h2>
    <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
      {#each PROVIDERS as p (p.name)}
        <button
          type="button"
          class="flex items-start gap-3 rounded-lg border bg-card p-4 text-left text-card-foreground shadow-sm transition-colors hover:bg-accent/50"
          disabled={submitting}
          onclick={() => handlePresetClick(p.name)}
        >
          <ProviderIcon name={p.name} class="size-8 shrink-0" />
          <div>
            <div class="font-semibold">{p.name}</div>
            <div class="text-sm text-muted-foreground">{p.description}</div>
          </div>
        </button>
      {/each}
    </div>
  </div>
</div>
