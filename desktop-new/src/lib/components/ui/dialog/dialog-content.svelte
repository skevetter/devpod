<script lang="ts">
import type { Dialog } from "bits-ui"
import { cn } from "$lib/utils.js"
import type { Snippet } from "svelte"

let {
  ref = $bindable(null),
  class: className,
  children,
  ...restProps
}: Dialog.ContentProps & {
  ref?: HTMLElement | null
  children?: Snippet
} = $props()
</script>

<Dialog.Portal>
  <Dialog.Overlay
    class="fixed inset-0 z-50 bg-black/80"
  />
  <Dialog.Content
    bind:ref
    class={cn(
      "fixed left-[50%] top-[50%] z-50 grid w-full max-w-lg translate-x-[-50%] translate-y-[-50%] gap-4 border bg-background p-6 shadow-lg sm:rounded-lg",
      className,
    )}
    {...restProps}
  >
    {@render children?.()}
  </Dialog.Content>
</Dialog.Portal>
