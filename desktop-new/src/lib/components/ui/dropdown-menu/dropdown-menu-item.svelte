<script lang="ts">
import type { DropdownMenu } from "bits-ui"
import { cn } from "$lib/utils.js"
import type { Snippet } from "svelte"

let {
  ref = $bindable(null),
  class: className,
  children,
  ...restProps
}: DropdownMenu.ItemProps & {
  ref?: HTMLElement | null
  children?: Snippet
} = $props()
</script>

<DropdownMenu.Item
  bind:ref
  class={cn(
    "relative flex cursor-default select-none items-center rounded-sm px-2 py-1.5 text-sm outline-none transition-colors data-[highlighted]:bg-accent data-[highlighted]:text-accent-foreground data-[disabled]:pointer-events-none data-[disabled]:opacity-50",
    className,
  )}
  {...restProps}
>
  {@render children?.()}
</DropdownMenu.Item>
