<script lang="ts">
import { page } from "$app/stores"
import { cn } from "$lib/utils.js"

let {
  href,
  label,
  badgeCount,
}: {
  href: string
  label: string
  badgeCount?: number
} = $props()

let isActive = $derived($page.url.pathname.startsWith(href))
</script>

<a
  {href}
  class={cn(
    "flex items-center justify-between rounded-md px-3 py-2 text-sm font-medium transition-colors",
    isActive
      ? "bg-accent text-accent-foreground"
      : "text-muted-foreground hover:bg-accent/50 hover:text-accent-foreground",
  )}
>
  <span>{label}</span>
  {#if badgeCount != null && badgeCount > 0}
    <span
      class="inline-flex items-center rounded-full bg-primary px-2 py-0.5 text-xs font-semibold text-primary-foreground"
    >
      {badgeCount}
    </span>
  {/if}
</a>
