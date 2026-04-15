<script lang="ts">
import type { Tooltip } from "bits-ui"
import { cn } from "$lib/utils.js"
import type { Snippet } from "svelte"

let {
  ref = $bindable(null),
  class: className,
  sideOffset = 4,
  children,
  ...restProps
}: Tooltip.ContentProps & {
  ref?: HTMLElement | null
  children?: Snippet
} = $props()
</script>

<Tooltip.Portal>
  <Tooltip.Content
    bind:ref
    class={cn(
      "z-50 overflow-hidden rounded-md border bg-popover px-3 py-1.5 text-sm text-popover-foreground shadow-md animate-in fade-in-0 zoom-in-95",
      className,
    )}
    {sideOffset}
    {...restProps}
  >
    {@render children?.()}
  </Tooltip.Content>
</Tooltip.Portal>
