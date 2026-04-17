<script lang="ts">
import { Select } from "bits-ui"
import { cn } from "$lib/utils.js"
import type { Snippet } from "svelte"

let {
  ref = $bindable(null),
  class: className,
  children,
  sideOffset = 4,
  ...restProps
}: Select.ContentProps & {
  ref?: HTMLElement | null
  children?: Snippet
} = $props()
</script>

<Select.Portal>
  <Select.Content
    bind:ref
    class={cn(
      "relative z-50 max-h-96 min-w-[var(--bits-select-trigger-width,8rem)] overflow-hidden rounded-md border bg-popover text-popover-foreground shadow-md animate-in fade-in-0 zoom-in-95",
      className,
    )}
    {sideOffset}
    {...restProps}
  >
    <Select.Viewport class="p-1">
      {@render children?.()}
    </Select.Viewport>
  </Select.Content>
</Select.Portal>
