<script lang="ts">
import { Button } from "$lib/components/ui/button/index.js"
import { badgeVariants } from "$lib/components/ui/badge/index.js"
import ConfirmDialog from "$lib/components/layout/ConfirmDialog.svelte"
import { providerUse, providerDelete } from "$lib/ipc/commands.js"
import { toasts } from "$lib/stores/toasts.js"
import type { Provider } from "$lib/types/index.js"
import { goto } from "$app/navigation"

let { provider }: { provider: Provider } = $props()
let confirmDeleteOpen = $state(false)
let deleting = $state(false)

function sourceDisplay(p: Provider): string {
  if (p.source?.github) return p.source.github
  if (p.source?.url) return p.source.url
  if (p.source?.file) return p.source.file
  return ""
}

async function handleSetDefault(e: Event) {
  e.stopPropagation()
  try {
    await providerUse(provider.name)
    toasts.success(`Set ${provider.name} as default`)
  } catch (err) {
    toasts.error(`Failed to set default: ${err}`)
  }
}

function openDeleteConfirm(e: Event) {
  e.stopPropagation()
  confirmDeleteOpen = true
}

async function handleDelete() {
  deleting = true
  try {
    await providerDelete(provider.name)
    toasts.success(`Deleted ${provider.name}`)
    confirmDeleteOpen = false
  } catch (err) {
    toasts.error(`Failed to delete: ${err}`)
  } finally {
    deleting = false
  }
}
</script>

<button
  type="button"
  class="rounded-lg border bg-card p-4 text-left text-card-foreground shadow-sm transition-colors hover:bg-accent/50 w-full"
  onclick={() => goto(`/providers/${provider.name}`)}
>
  <div class="flex items-start justify-between gap-2">
    <h3 class="font-semibold truncate">{provider.name}</h3>
    <div class="flex gap-1">
      {#if provider.state?.initialized}
        <span class={badgeVariants({ variant: "default" })}>initialized</span>
      {/if}
      {#if provider.version}
        <span class={badgeVariants({ variant: "secondary" })}>{provider.version}</span>
      {/if}
    </div>
  </div>

  {#if provider.description}
    <p class="mt-1 text-sm text-muted-foreground truncate">{provider.description}</p>
  {/if}

  {#if sourceDisplay(provider)}
    <p class="mt-1 text-xs text-muted-foreground truncate">{sourceDisplay(provider)}</p>
  {/if}

  <div class="mt-3 flex gap-2">
    <Button variant="default" size="sm" onclick={(e) => { e.stopPropagation(); goto(`/providers/${provider.name}`) }}>Edit</Button>
    <Button variant="outline" size="sm" onclick={handleSetDefault}>Set Default</Button>
    <Button variant="destructive" size="sm" onclick={openDeleteConfirm}>Delete</Button>
  </div>
</button>

<ConfirmDialog
  bind:open={confirmDeleteOpen}
  title="Delete provider"
  description="This will remove provider '{provider.name}' and its configuration. Any workspaces using this provider will need a new one."
  confirmLabel="Delete"
  loading={deleting}
  onconfirm={handleDelete}
/>
