<script lang="ts">
import { Bell, CheckCircle, XCircle, Info, Trash2 } from "lucide-svelte"
import { Button } from "$lib/components/ui/button/index.js"
import { ScrollArea } from "$lib/components/ui/scroll-area/index.js"
import { notificationHistory } from "$lib/stores/toasts.js"

const unreadCount = notificationHistory.unreadCount

let open = $state(false)
let panelEl = $state<HTMLDivElement | null>(null)

function handleClickOutside(e: MouseEvent) {
  if (panelEl && !panelEl.contains(e.target as Node)) {
    open = false
  }
}

function formatTime(ts: number): string {
  const d = new Date(ts)
  return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
}
</script>

<svelte:window onclick={handleClickOutside} />

<div class="relative" bind:this={panelEl}>
  <Button
    variant="ghost"
    size="sm"
    class="relative"
    onclick={(e) => { e.stopPropagation(); open = !open }}
  >
    <Bell class="h-4 w-4" />
    {#if $unreadCount > 0}
      <span class="absolute -top-0.5 -right-0.5 flex h-4 w-4 items-center justify-center rounded-full bg-destructive text-[10px] font-bold text-destructive-foreground">
        {$unreadCount > 9 ? "9+" : $unreadCount}
      </span>
    {/if}
  </Button>

  {#if open}
    <div class="absolute right-0 top-full mt-2 w-80 rounded-md border shadow-lg z-50 text-popover-foreground" style="background-color: hsl(var(--popover) / 1);">
      <div class="flex items-center justify-between border-b px-3 py-2">
        <span class="text-sm font-semibold">Notifications</span>
        {#if $notificationHistory.length > 0}
          <Button variant="ghost" size="sm" onclick={() => notificationHistory.clear()}>
            <Trash2 class="h-3 w-3" />
          </Button>
        {/if}
      </div>

      {#if $notificationHistory.length === 0}
        <div class="px-3 py-6 text-center text-sm text-muted-foreground">
          No notifications yet
        </div>
      {:else}
        <ScrollArea class="max-h-80">
          <div class="divide-y">
            {#each $notificationHistory as item (item.id)}
              <div class="flex items-start gap-2 px-3 py-2.5">
                {#if item.variant === "success"}
                  <CheckCircle class="mt-0.5 h-4 w-4 shrink-0 text-green-500" />
                {:else if item.variant === "error"}
                  <XCircle class="mt-0.5 h-4 w-4 shrink-0 text-destructive" />
                {:else}
                  <Info class="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
                {/if}
                <div class="min-w-0 flex-1">
                  <p class="text-sm break-words {item.variant === 'error' ? 'text-destructive' : ''}">{item.message}</p>
                  <p class="text-xs text-muted-foreground">{formatTime(item.timestamp)}</p>
                </div>
              </div>
            {/each}
          </div>
        </ScrollArea>
      {/if}
    </div>
  {/if}
</div>
