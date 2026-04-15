<script lang="ts">
import { Button } from "bits-ui"
import type { Snippet } from "svelte"
import { cn } from "$lib/utils.js"
import { buttonVariants, type Props } from "./index.js"

let {
  ref = $bindable(null),
  class: className,
  variant = "default",
  size = "default",
  children,
  ...restProps
}: Props & {
  ref?: HTMLButtonElement | null
  children?: Snippet
} = $props()
</script>

<Button.Root
	bind:ref
	class={cn(buttonVariants({ variant, size, className }))}
	{...restProps}
>
	{@render children?.()}
</Button.Root>
