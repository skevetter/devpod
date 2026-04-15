<script lang="ts">
import type { DropdownMenu } from "bits-ui"
import { cn } from "$lib/utils.js"
import type { Snippet } from "svelte"

let {
  ref = $bindable(null),
  class: className,
  children,
  ...restProps
}: DropdownMenu.GroupHeadingProps & {
  ref?: HTMLElement | null
  children?: Snippet
} = $props()
</script>

<DropdownMenu.GroupHeading
  bind:ref
  class={cn("px-2 py-1.5 text-sm font-semibold", className)}
  {...restProps}
>
  {@render children?.()}
</DropdownMenu.GroupHeading>
