<script lang="ts">
import { Button } from "$lib/components/ui/button/index.js"
import { badgeVariants } from "$lib/components/ui/badge/index.js"
import {
  contexts,
  activeContext,
  contextsLoading,
} from "$lib/stores/contexts.js"
import { contextUse } from "$lib/ipc/commands.js"
import { toasts } from "$lib/stores/toasts.js"
import { Layers } from "lucide-svelte"

async function handleUse(name: string) {
  try {
    await contextUse(name)
    toasts.success(`Switched to context "${name}"`)
  } catch (err) {
    toasts.error(`Failed to switch context: ${err}`)
  }
}
</script>

<div class="space-y-6">
  <div class="flex items-center justify-between">
    <h1 class="text-2xl font-bold">Contexts</h1>
  </div>

  {#if $contextsLoading}
    <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
      {#each Array(2) as _}
        <div class="h-28 animate-pulse rounded-lg border bg-muted"></div>
      {/each}
    </div>
  {:else if $contexts.length === 0}
    <div class="flex flex-col items-center justify-center gap-4 py-16 text-center">
      <Layers class="h-10 w-10 text-muted-foreground" />
      <p class="text-muted-foreground">No contexts found.</p>
    </div>
  {:else}
    <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
      {#each $contexts as ctx (ctx.name)}
        <div
          class="rounded-lg border bg-card p-4 text-card-foreground shadow-sm {ctx.name === $activeContext ? 'ring-2 ring-primary' : ''}"
        >
          <div class="flex items-start justify-between gap-2">
            <h3 class="font-semibold">{ctx.name}</h3>
            {#if ctx.name === $activeContext}
              <span class={badgeVariants({ variant: "default" })}>active</span>
            {/if}
          </div>

          {#if ctx.options && Object.keys(ctx.options).length > 0}
            <div class="mt-2 space-y-1">
              {#each Object.entries(ctx.options) as [key, value]}
                <div class="text-xs text-muted-foreground">
                  <span class="font-medium">{key}:</span>
                  {value}
                </div>
              {/each}
            </div>
          {/if}

          {#if ctx.name !== $activeContext}
            <div class="mt-3">
              <Button variant="outline" size="sm" onclick={() => handleUse(ctx.name)}>
                Use
              </Button>
            </div>
          {/if}
        </div>
      {/each}
    </div>
  {/if}
</div>
