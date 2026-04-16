<script lang="ts">
import { toasts } from "$lib/stores/toasts.js"
import { fly } from "svelte/transition"
import { CheckCircle, XCircle, Info, X } from "lucide-svelte"
</script>

{#if $toasts.length > 0}
  <div class="fixed bottom-4 right-4 z-50 flex flex-col gap-2">
    {#each $toasts as toast (toast.id)}
      <button
        type="button"
        class="flex items-center gap-3 rounded-md border px-4 py-3 text-sm shadow-lg {toast.variant === 'error'
          ? 'border-destructive bg-destructive/10 text-destructive'
          : toast.variant === 'success'
            ? 'border-green-500 bg-green-500/10 text-green-700 dark:text-green-400'
            : 'border-border bg-card text-card-foreground'}"
        onclick={() => toasts.dismiss(toast.id)}
        transition:fly={{ x: 100, duration: 200 }}
      >
        {#if toast.variant === "success"}
          <CheckCircle class="h-4 w-4 shrink-0" />
        {:else if toast.variant === "error"}
          <XCircle class="h-4 w-4 shrink-0" />
        {:else}
          <Info class="h-4 w-4 shrink-0" />
        {/if}
        <span class="flex-1">{toast.message}</span>
        <X class="h-3 w-3 shrink-0 opacity-50" />
      </button>
    {/each}
  </div>
{/if}
