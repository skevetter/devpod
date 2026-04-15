<script lang="ts">
import type { Tabs } from "bits-ui"
import { cn } from "$lib/utils.js"
import type { Snippet } from "svelte"

let {
  ref = $bindable(null),
  class: className,
  value,
  children,
  ...restProps
}: Tabs.ContentProps & {
  ref?: HTMLElement | null
  children?: Snippet
} = $props()
</script>

<Tabs.Content
  bind:ref
  class={cn(
    "mt-2 ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2",
    className,
  )}
  {value}
  {...restProps}
>
  {@render children?.()}
</Tabs.Content>
