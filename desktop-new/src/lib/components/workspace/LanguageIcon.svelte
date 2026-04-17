<script lang="ts">
import { MediaQuery } from "svelte/reactivity"

let { name, class: className = "size-10" }: { name: string; class?: string } =
  $props()

const darkMode = new MediaQuery("(prefers-color-scheme: dark)")

const ICON_MAP: Record<string, string> = {
  python: "/icons/languages/python.svg",
  node: "/icons/languages/nodejs.svg",
  "node.js": "/icons/languages/nodejs.svg",
  nodejs: "/icons/languages/nodejs.svg",
  javascript: "/icons/languages/nodejs.svg",
  go: "/icons/languages/go.svg",
  golang: "/icons/languages/go.svg",
  rust: "/icons/languages/rust.svg",
  java: "/icons/languages/java.svg",
  php: "/icons/languages/php.svg",
  "c++": "/icons/languages/cpp.svg",
  cpp: "/icons/languages/cpp.svg",
  ".net": "/icons/languages/dotnet.svg",
  dotnet: "/icons/languages/dotnet.svg",
  "c#": "/icons/languages/dotnet.svg",
  csharp: "/icons/languages/dotnet.svg",
  ruby: "/icons/languages/ruby.svg",
}

const DARK_VARIANTS: Record<string, string> = {
  "/icons/languages/go.svg": "/icons/languages/go_dark.svg",
  "/icons/languages/rust.svg": "/icons/languages/rust_dark.svg",
  "/icons/languages/php.svg": "/icons/languages/php_dark.svg",
}

const src = $derived.by(() => {
  const lower = name.toLowerCase()
  let icon: string | null = null
  if (ICON_MAP[lower]) {
    icon = ICON_MAP[lower]
  } else {
    for (const [key, value] of Object.entries(ICON_MAP)) {
      if (lower.includes(key)) {
        icon = value
        break
      }
    }
  }
  if (icon && darkMode.current && DARK_VARIANTS[icon]) {
    return DARK_VARIANTS[icon]
  }
  return icon
})
</script>

{#if src}
  <img {src} alt="{name} icon" class={className} />
{:else}
  <span
    class="flex items-center justify-center rounded-md bg-muted text-xs font-bold {className}"
  >
    {name.slice(0, 2).toUpperCase()}
  </span>
{/if}
