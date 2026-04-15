<script lang="ts">
import type { Dialog } from "bits-ui"
import { cn } from "$lib/utils.js"
import type { Snippet } from "svelte"

let {
  ref = $bindable(null),
  class: className,
  children,
  ...restProps
}: Dialog.TitleProps & {
  ref?: HTMLElement | null
  children?: Snippet
} = $props()
</script>

<Dialog.Title
  bind:ref
  class={cn("text-lg font-semibold leading-none tracking-tight", className)}
  {...restProps}
>
  {@render children?.()}
</Dialog.Title>
