<script lang="ts">
import { goto } from "$app/navigation"
import { Button } from "$lib/components/ui/button/index.js"
import { Input } from "$lib/components/ui/input/index.js"
import { Label } from "$lib/components/ui/label/index.js"
import { providerAdd } from "$lib/ipc/commands.js"
import { toasts } from "$lib/stores/toasts.js"
import {
  Container,
  Terminal,
  Ship,
  Cloud,
  Globe,
  Server,
  Droplets,
} from "lucide-svelte"
import type { Component } from "svelte"

const POPULAR_PROVIDERS: {
  name: string
  description: string
  icon: Component<{ class?: string }>
}[] = [
  { name: "docker", description: "Local Docker containers", icon: Container },
  { name: "ssh", description: "Remote SSH machines", icon: Terminal },
  { name: "kubernetes", description: "Kubernetes clusters", icon: Ship },
  { name: "aws", description: "Amazon Web Services", icon: Cloud },
  { name: "gcloud", description: "Google Cloud Platform", icon: Globe },
  { name: "azure", description: "Microsoft Azure", icon: Server },
  {
    name: "digitalocean",
    description: "DigitalOcean Droplets",
    icon: Droplets,
  },
]

let providerSource = $state("")
let error = $state("")
let submitting = $state(false)

async function handleAdd(name: string, source?: string) {
  error = ""
  submitting = true
  try {
    await providerAdd(name, source)
    toasts.success(`Added provider ${name}`)
    goto("/providers")
  } catch (err) {
    error = err instanceof Error ? err.message : String(err)
  } finally {
    submitting = false
  }
}

async function handleSubmit() {
  if (!providerSource.trim()) {
    error = "Provider name or source is required"
    return
  }
  await handleAdd(providerSource.trim())
}
</script>

<div class="mx-auto max-w-xl space-y-6">
  <div class="flex items-center gap-4">
    <Button variant="ghost" size="sm" onclick={() => goto("/providers")}>
      &larr; Back
    </Button>
    <h1 class="text-2xl font-bold">Add Provider</h1>
  </div>

  {#if error}
    <div class="rounded-md border border-destructive bg-destructive/10 p-3 text-sm text-destructive">
      {error}
    </div>
  {/if}

  <form class="space-y-4" onsubmit={(e) => { e.preventDefault(); handleSubmit() }}>
    <div class="space-y-2">
      <Label>Provider Name or Source</Label>
      <Input
        placeholder="e.g. docker, or github.com/org/provider"
        value={providerSource}
        oninput={(e) => (providerSource = e.currentTarget.value)}
      />
    </div>

    <Button type="submit" disabled={submitting} class="w-full">
      {submitting ? "Adding..." : "Add Provider"}
    </Button>
  </form>

  <div class="space-y-3">
    <h2 class="text-lg font-semibold">Popular Providers</h2>
    <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
      {#each POPULAR_PROVIDERS as p (p.name)}
        {@const Icon = p.icon}
        <button
          type="button"
          class="flex items-start gap-3 rounded-lg border bg-card p-4 text-left text-card-foreground shadow-sm transition-colors hover:bg-accent/50"
          disabled={submitting}
          onclick={() => handleAdd(p.name)}
        >
          <Icon class="h-5 w-5 mt-0.5 shrink-0 text-muted-foreground" />
          <div>
            <div class="font-semibold">{p.name}</div>
            <div class="text-sm text-muted-foreground">{p.description}</div>
          </div>
        </button>
      {/each}
    </div>
  </div>
</div>
