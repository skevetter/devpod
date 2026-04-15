<script lang="ts">
// biome-ignore lint/correctness/noUnusedImports: Button is used as namespace in template
import { Button } from "bits-ui"
import type { Snippet } from "svelte"
// biome-ignore lint/correctness/noUnusedImports: cn is used in template
import { cn } from "$lib/utils.js"
// biome-ignore lint/correctness/noUnusedImports: buttonVariants is used in template
import { buttonVariants, type Props } from "./index.js"

let {
  ref = $bindable(null),
  class: className,
  variant = "default",
  size = "default",
  children,
  // biome-ignore lint/correctness/noUnusedVariables: restProps is spread in template
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
