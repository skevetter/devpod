<script lang="ts">
import type { Label } from "bits-ui"
import { cn } from "$lib/utils.js"
import type { Snippet } from "svelte"

let {
  ref = $bindable(null),
  class: className,
  children,
  ...restProps
}: Label.RootProps & {
  ref?: HTMLElement | null
  children?: Snippet
} = $props()
</script>

<Label.Root
  bind:ref
  class={cn(
    "text-sm font-medium leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-70",
    className,
  )}
  {...restProps}
>
  {@render children?.()}
</Label.Root>
