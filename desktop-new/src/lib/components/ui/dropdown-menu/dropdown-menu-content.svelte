<script lang="ts">
import type { DropdownMenu } from "bits-ui"
import { cn } from "$lib/utils.js"
import type { Snippet } from "svelte"

let {
  ref = $bindable(null),
  class: className,
  sideOffset = 4,
  children,
  ...restProps
}: DropdownMenu.ContentProps & {
  ref?: HTMLElement | null
  children?: Snippet
} = $props()
</script>

<DropdownMenu.Portal>
  <DropdownMenu.Content
    bind:ref
    class={cn(
      "z-50 min-w-[8rem] overflow-hidden rounded-md border bg-popover p-1 text-popover-foreground shadow-md animate-in fade-in-0 zoom-in-95",
      className,
    )}
    {sideOffset}
    {...restProps}
  >
    {@render children?.()}
  </DropdownMenu.Content>
</DropdownMenu.Portal>
