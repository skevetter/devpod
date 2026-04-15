<script lang="ts">
import type { Select } from "bits-ui"
import { cn } from "$lib/utils.js"
import type { Snippet } from "svelte"

let {
  ref = $bindable(null),
  class: className,
  children,
  ...restProps
}: Select.TriggerProps & {
  ref?: HTMLElement | null
  children?: Snippet
} = $props()
</script>

<Select.Trigger
  bind:ref
  class={cn(
    "flex h-10 w-full items-center justify-between rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 [&>span]:line-clamp-1",
    className,
  )}
  {...restProps}
>
  {@render children?.()}
</Select.Trigger>
