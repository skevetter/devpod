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
}: Tabs.TriggerProps & {
  ref?: HTMLElement | null
  children?: Snippet
} = $props()
</script>

<Tabs.Trigger
  bind:ref
  class={cn(
    "inline-flex items-center justify-center whitespace-nowrap rounded-sm px-3 py-1.5 text-sm font-medium ring-offset-background transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:pointer-events-none disabled:opacity-50 data-[state=active]:bg-background data-[state=active]:text-foreground data-[state=active]:shadow-sm",
    className,
  )}
  {value}
  {...restProps}
>
  {@render children?.()}
</Tabs.Trigger>
