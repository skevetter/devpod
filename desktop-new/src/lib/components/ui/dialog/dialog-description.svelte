<script lang="ts">
import type { Dialog } from "bits-ui"
import { cn } from "$lib/utils.js"
import type { Snippet } from "svelte"

let {
  ref = $bindable(null),
  class: className,
  children,
  ...restProps
}: Dialog.DescriptionProps & {
  ref?: HTMLElement | null
  children?: Snippet
} = $props()
</script>

<Dialog.Description
  bind:ref
  class={cn("text-sm text-muted-foreground", className)}
  {...restProps}
>
  {@render children?.()}
</Dialog.Description>
