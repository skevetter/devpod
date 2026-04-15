<script lang="ts">
import type { Tabs } from "bits-ui"
import { cn } from "$lib/utils.js"
import type { Snippet } from "svelte"

let {
  ref = $bindable(null),
  class: className,
  children,
  ...restProps
}: Tabs.ListProps & {
  ref?: HTMLElement | null
  children?: Snippet
} = $props()
</script>

<Tabs.List
  bind:ref
  class={cn(
    "inline-flex h-10 items-center justify-center rounded-md bg-muted p-1 text-muted-foreground",
    className,
  )}
  {...restProps}
>
  {@render children?.()}
</Tabs.List>
