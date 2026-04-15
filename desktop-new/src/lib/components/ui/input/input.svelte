<script lang="ts">
import type { HTMLInputAttributes } from "svelte/elements"
import { cn } from "$lib/utils.js"

let {
  ref = $bindable(null),
  value = $bindable(""),
  class: className,
  type = "text",
  ...restProps
}: HTMLInputAttributes & {
  ref?: HTMLInputElement | null
  value?: string
} = $props()
</script>

<input
  bind:this={ref}
  bind:value
  {type}
  class={cn(
    "flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background file:border-0 file:bg-transparent file:text-sm file:font-medium file:text-foreground placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50",
    className,
  )}
  {...restProps}
/>
