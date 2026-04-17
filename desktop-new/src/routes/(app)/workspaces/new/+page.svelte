<script lang="ts">
import { goto } from "$app/navigation"
import { onDestroy } from "svelte"
import { Check, ChevronsUpDown } from "@lucide/svelte"
import { Button } from "$lib/components/ui/button/index.js"
import * as Command from "$lib/components/ui/command/index.js"
import { Input } from "$lib/components/ui/input/index.js"
import { Label } from "$lib/components/ui/label/index.js"
import * as Popover from "$lib/components/ui/popover/index.js"
import { ScrollArea } from "$lib/components/ui/scroll-area/index.js"
import { workspaceUp } from "$lib/ipc/commands.js"
import { onCommandProgress } from "$lib/ipc/events.js"
import { providers } from "$lib/stores/providers.js"
import { toasts } from "$lib/stores/toasts.js"
import { stripAnsi } from "$lib/utils/log-parser.js"
import type { UnlistenFn } from "@tauri-apps/api/event"

const IDE_GROUPS = [
  {
    label: "Primary",
    options: [
      { value: "none", label: "None" },
      { value: "vscode", label: "VS Code" },
      { value: "openvscode", label: "VS Code Browser" },
      { value: "cursor", label: "Cursor" },
      { value: "zed", label: "Zed" },
      { value: "codium", label: "VSCodium" },
      { value: "windsurf", label: "Windsurf Editor" },
      { value: "antigravity", label: "Google Antigravity" },
      { value: "bob", label: "IBM Bob" },
    ],
  },
  {
    label: "JetBrains",
    options: [
      { value: "intellij", label: "IntelliJ IDEA" },
      { value: "pycharm", label: "PyCharm" },
      { value: "phpstorm", label: "PhpStorm" },
      { value: "rider", label: "Rider" },
      { value: "fleet", label: "Fleet" },
      { value: "goland", label: "GoLand" },
      { value: "webstorm", label: "WebStorm" },
      { value: "rustrover", label: "RustRover" },
      { value: "rubymine", label: "RubyMine" },
      { value: "clion", label: "CLion" },
      { value: "dataspell", label: "DataSpell" },
    ],
  },
  {
    label: "Other",
    options: [
      { value: "jupyternotebook", label: "Jupyter Notebook" },
      { value: "vscode-insiders", label: "VS Code Insiders" },
      { value: "positron", label: "Positron" },
      { value: "rstudio", label: "RStudio Server" },
    ],
  },
]

const ALL_IDES = IDE_GROUPS.flatMap((g) => g.options)

const TEMPLATES = [
  {
    name: "Python",
    source: "https://github.com/microsoft/vscode-remote-try-python",
    icon: "PY",
  },
  {
    name: "Node.js",
    source: "https://github.com/microsoft/vscode-remote-try-node",
    icon: "JS",
  },
  {
    name: "Go",
    source: "https://github.com/microsoft/vscode-remote-try-go",
    icon: "GO",
  },
  {
    name: "Rust",
    source: "https://github.com/microsoft/vscode-remote-try-rust",
    icon: "RS",
  },
  {
    name: "Java",
    source: "https://github.com/microsoft/vscode-remote-try-java",
    icon: "JV",
  },
  {
    name: "PHP",
    source: "https://github.com/microsoft/vscode-remote-try-php",
    icon: "PHP",
  },
  {
    name: "C++",
    source: "https://github.com/microsoft/vscode-remote-try-cpp",
    icon: "C++",
  },
  {
    name: ".NET",
    source: "https://github.com/microsoft/vscode-remote-try-dotnet",
    icon: "C#",
  },
  {
    name: "Ruby",
    source: "https://github.com/skevetter/devpod-quickstart-ruby",
    icon: "RB",
  },
]

let source = $state("")
let name = $state("")
let selectedProvider = $state("")
let selectedIde = $state("")

let providerComboOpen = $state(false)
let providerSearch = $state("")
let ideComboOpen = $state(false)
let ideSearch = $state("")

const providerLabel = $derived(selectedProvider || "Select a provider...")
const ideLabel = $derived(
  ALL_IDES.find((i) => i.value === selectedIde)?.label ?? "Select an IDE...",
)

let filteredProviders = $derived(
  providerSearch
    ? $providers.filter((p) =>
        p.name.toLowerCase().includes(providerSearch.toLowerCase()),
      )
    : $providers,
)

let filteredIdes = $derived(
  ideSearch
    ? ALL_IDES.filter((i) =>
        i.label.toLowerCase().includes(ideSearch.toLowerCase()),
      )
    : ALL_IDES,
)

// Auto-select if only one provider is available
$effect(() => {
  if (!selectedProvider && $providers.length === 1) {
    selectedProvider = $providers[0].name
  }
})
let error = $state("")
let submitting = $state(false)

let commandId = $state<string | null>(null)
let outputLines = $state<string[]>([])
let outputEl = $state<HTMLPreElement | null>(null)
let unlisten: UnlistenFn | null = null

onDestroy(() => {
  unlisten?.()
})

async function handleSubmit() {
  if (!source.trim()) {
    error = "Source is required"
    return
  }

  error = ""
  submitting = true
  outputLines = []

  try {
    const workspaceId =
      name.trim() ||
      source
        .trim()
        .split("/")
        .pop()
        ?.replace(/\.git$/, "") ||
      undefined

    // Subscribe to progress events before starting
    unlisten = await onCommandProgress((progress) => {
      if (commandId && progress.commandId === commandId) {
        if (progress.message) {
          outputLines = [...outputLines, progress.message]
          requestAnimationFrame(() => {
            outputEl?.scrollIntoView({ block: "end", behavior: "smooth" })
          })
        }
        if (progress.done) {
          submitting = false
          if (stripAnsi(progress.message).includes("Exit code: 0")) {
            toasts.success(`Workspace ${workspaceId ?? "created"} is ready`)
            goto(`/workspaces/${workspaceId}`)
          } else {
            toasts.error("Workspace creation failed. Check output for details.")
          }
        }
      }
    })

    commandId = await workspaceUp({
      source: source.trim(),
      workspaceId,
      provider: selectedProvider || undefined,
      ide: selectedIde || undefined,
    })
  } catch (err) {
    toasts.error(`Failed to create workspace: ${err}`)
    submitting = false
  }
}
</script>

<div class="mx-auto max-w-xl space-y-6">
  <div class="flex items-center gap-4">
    <Button variant="ghost" size="sm" onclick={() => goto("/workspaces")}>
      &larr; Back
    </Button>
    <h1 class="text-2xl font-bold">Create Workspace</h1>
  </div>

  <div class="space-y-3">
    <h2 class="text-sm font-semibold">Quick Start Templates</h2>
    <div class="grid grid-cols-3 gap-2 sm:grid-cols-4">
      {#each TEMPLATES as template (template.name)}
        <button
          type="button"
          class="flex flex-col items-center gap-1.5 rounded-lg border bg-card p-3 text-center text-sm transition-colors hover:bg-accent/50 active:scale-[0.98] {source === template.source ? 'border-primary ring-1 ring-primary' : ''}"
          onclick={() => { source = template.source; name = template.name.toLowerCase().replace(/[^a-z0-9]/g, '-') }}
        >
          <span class="flex h-8 w-8 items-center justify-center rounded-md bg-muted text-xs font-bold">{template.icon}</span>
          <span class="truncate text-xs">{template.name}</span>
        </button>
      {/each}
    </div>
  </div>

  {#if error}
    <div class="rounded-md border border-destructive bg-destructive/10 p-3 text-sm text-destructive">
      {error}
    </div>
  {/if}

  <form class="space-y-4" onsubmit={(e) => { e.preventDefault(); handleSubmit() }}>
    <div class="space-y-2">
      <Label>Source *</Label>
      <Input
        placeholder="github.com/org/repo, local path, or image"
        value={source}
        oninput={(e) => (source = e.currentTarget.value)}
        disabled={submitting}
      />
    </div>

    <div class="space-y-2">
      <Label>Workspace Name</Label>
      <Input
        placeholder="Optional - derived from source if empty"
        value={name}
        oninput={(e) => (name = e.currentTarget.value)}
        disabled={submitting}
      />
    </div>

    <div class="space-y-2">
      <Label>Provider</Label>
      <Popover.Root bind:open={providerComboOpen}>
        <Popover.Trigger class="w-full">
          {#snippet child({ props })}
            <Button variant="outline" class="w-full justify-between" {...props} disabled={submitting}>
              <span class="truncate text-left">{providerLabel}</span>
              <ChevronsUpDown class="ml-2 h-4 w-4 shrink-0 opacity-50" />
            </Button>
          {/snippet}
        </Popover.Trigger>
        <Popover.Content class="w-[var(--bits-popover-anchor-width)] p-0" align="start">
          <Command.Root shouldFilter={false}>
            <Command.Input placeholder="Search providers..." bind:value={providerSearch} />
            <Command.List class="max-h-60">
              <Command.Empty>No provider found.</Command.Empty>
              <Command.Group>
                {#each filteredProviders as p (p.name)}
                  <Command.Item
                    value={p.name}
                    class="justify-start"
                    onSelect={() => { selectedProvider = p.name; providerComboOpen = false; providerSearch = "" }}
                  >
                    <Check class="mr-2 h-4 w-4 {selectedProvider === p.name ? 'opacity-100' : 'opacity-0'}" />
                    {p.name}
                  </Command.Item>
                {/each}
              </Command.Group>
            </Command.List>
          </Command.Root>
        </Popover.Content>
      </Popover.Root>
    </div>

    <div class="space-y-2">
      <Label>IDE</Label>
      <Popover.Root bind:open={ideComboOpen}>
        <Popover.Trigger class="w-full">
          {#snippet child({ props })}
            <Button variant="outline" class="w-full justify-between" {...props} disabled={submitting}>
              <span class="truncate text-left">{ideLabel}</span>
              <ChevronsUpDown class="ml-2 h-4 w-4 shrink-0 opacity-50" />
            </Button>
          {/snippet}
        </Popover.Trigger>
        <Popover.Content class="w-[var(--bits-popover-anchor-width)] p-0" align="start">
          <Command.Root shouldFilter={false}>
            <Command.Input placeholder="Search IDEs..." bind:value={ideSearch} />
            <Command.List class="max-h-60">
              <Command.Empty>No IDE found.</Command.Empty>
              <Command.Group>
                {#each filteredIdes as ide (ide.value)}
                  <Command.Item
                    value={ide.value}
                    class="justify-start"
                    onSelect={() => { selectedIde = ide.value; ideComboOpen = false; ideSearch = "" }}
                  >
                    <Check class="mr-2 h-4 w-4 {selectedIde === ide.value ? 'opacity-100' : 'opacity-0'}" />
                    {ide.label}
                  </Command.Item>
                {/each}
              </Command.Group>
            </Command.List>
          </Command.Root>
        </Popover.Content>
      </Popover.Root>
    </div>

    <div class="pt-2">
      <Button type="submit" disabled={submitting} class="w-full">
        {submitting ? "Creating..." : "Create Workspace"}
      </Button>
    </div>
  </form>

  {#if outputLines.length > 0}
    <div class="space-y-2">
      <div class="flex items-center justify-between">
        <h2 class="text-sm font-medium">Output</h2>
        <Button
          variant="outline"
          size="sm"
          onclick={async () => {
            try {
              await navigator.clipboard.writeText(outputLines.map(stripAnsi).join("\n"))
              toasts.success("Copied to clipboard")
            } catch {
              toasts.error("Failed to copy")
            }
          }}
        >
          Copy
        </Button>
      </div>
      <ScrollArea class="h-64 rounded-md border bg-muted/50 p-4">
        <pre bind:this={outputEl} class="text-xs font-mono whitespace-pre-wrap">{outputLines.map(stripAnsi).join("\n")}</pre>
      </ScrollArea>
    </div>
  {/if}
</div>
