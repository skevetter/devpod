<script lang="ts">
import type { ScrollArea } from "bits-ui"
import { cn } from "$lib/utils.js"
import type { Snippet } from "svelte"

let {
  ref = $bindable(null),
  class: className,
  orientation = "vertical",
  children,
  ...restProps
}: ScrollArea.RootProps & {
  ref?: HTMLElement | null
  children?: Snippet
} = $props()
</script>

<ScrollArea.Root
  bind:ref
  class={cn("relative overflow-hidden", className)}
  {...restProps}
>
  <ScrollArea.Viewport class="h-full w-full rounded-[inherit]">
    {@render children?.()}
  </ScrollArea.Viewport>
  <ScrollArea.Scrollbar
    orientation={orientation}
    class={cn(
      "flex touch-none select-none transition-colors",
      orientation === "vertical" && "h-full w-2.5 border-l border-l-transparent p-[1px]",
      orientation === "horizontal" && "h-2.5 flex-col border-t border-t-transparent p-[1px]",
    )}
  >
    <ScrollArea.Thumb
      class="relative flex-1 rounded-full bg-border"
    />
  </ScrollArea.Scrollbar>
  <ScrollArea.Corner />
</ScrollArea.Root>
