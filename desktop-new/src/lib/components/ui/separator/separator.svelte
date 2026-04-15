<script lang="ts">
import type { Separator } from "bits-ui"
import { cn } from "$lib/utils.js"

let {
  ref = $bindable(null),
  class: className,
  orientation = "horizontal",
  decorative = true,
  ...restProps
}: Separator.RootProps & {
  ref?: HTMLElement | null
} = $props()
</script>

<Separator.Root
  bind:ref
  class={cn(
    "shrink-0 bg-border",
    orientation === "horizontal" ? "h-[1px] w-full" : "h-full w-[1px]",
    className,
  )}
  {orientation}
  {decorative}
  {...restProps}
/>
