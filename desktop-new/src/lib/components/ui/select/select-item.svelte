<script lang="ts">
import type { Select } from "bits-ui"
import { cn } from "$lib/utils.js"
import type { Snippet } from "svelte"

let {
  ref = $bindable(null),
  class: className,
  value,
  label,
  children,
  ...restProps
}: Select.ItemProps & {
  ref?: HTMLElement | null
  children?: Snippet
} = $props()
</script>

<Select.Item
  bind:ref
  class={cn(
    "relative flex w-full cursor-default select-none items-center rounded-sm py-1.5 pl-8 pr-2 text-sm outline-none data-[highlighted]:bg-accent data-[highlighted]:text-accent-foreground data-[disabled]:pointer-events-none data-[disabled]:opacity-50",
    className,
  )}
  {value}
  {label}
  {...restProps}
>
  {#if children}
    {@render children()}
  {:else}
    {label ?? value}
  {/if}
</Select.Item>
