<script lang="ts">
import type { DropdownMenu } from "bits-ui"
import { cn } from "$lib/utils.js"

let {
  ref = $bindable(null),
  class: className,
  ...restProps
}: DropdownMenu.SeparatorProps & {
  ref?: HTMLElement | null
} = $props()
</script>

<DropdownMenu.Separator
  bind:ref
  class={cn("-mx-1 my-1 h-px bg-muted", className)}
  {...restProps}
/>
