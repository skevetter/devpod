<script lang="ts">
import { goto } from "$app/navigation"
import { onDestroy } from "svelte"
import { Button } from "$lib/components/ui/button/index.js"
import { Input } from "$lib/components/ui/input/index.js"
import { Label } from "$lib/components/ui/label/index.js"
import * as Select from "$lib/components/ui/select/index.js"
import { ScrollArea } from "$lib/components/ui/scroll-area/index.js"
import { workspaceUp } from "$lib/ipc/commands.js"
import { onCommandProgress } from "$lib/ipc/events.js"
import { providers } from "$lib/stores/providers.js"
import { toasts } from "$lib/stores/toasts.js"
import type { UnlistenFn } from "@tauri-apps/api/event"

const IDE_OPTIONS = [
  { value: "vscode", label: "VS Code" },
  { value: "openvscode", label: "OpenVSCode" },
  { value: "intellij", label: "IntelliJ" },
  { value: "goland", label: "GoLand" },
  { value: "pycharm", label: "PyCharm" },
  { value: "fleet", label: "Fleet" },
  { value: "jupyternotebook", label: "Jupyter Notebook" },
  { value: "none", label: "None" },
]

let source = $state("")
let name = $state("")
let selectedProvider = $state<string | undefined>(undefined)
let selectedIde = $state<string | undefined>(undefined)
let error = $state("")
let submitting = $state(false)

let commandId = $state<string | null>(null)
let outputLines = $state<string[]>([])
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
        }
        if (progress.done) {
          submitting = false
          if (progress.message.includes("Exit code: 0")) {
            toasts.success(`Workspace ${workspaceId ?? "created"} is ready`)
            goto(`/workspaces/${workspaceId}`)
          } else {
            error = "Workspace creation failed. Check output for details."
          }
        }
      }
    })

    commandId = await workspaceUp({
      source: source.trim(),
      workspaceId,
      provider: selectedProvider,
      ide: selectedIde,
    })
  } catch (err) {
    error = err instanceof Error ? err.message : String(err)
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
      <Select.Root type="single" onValueChange={(v) => (selectedProvider = v)} disabled={submitting}>
        <Select.Trigger>
          {#if selectedProvider}
            <span>{selectedProvider}</span>
          {:else}
            <span class="text-muted-foreground">Select a provider</span>
          {/if}
        </Select.Trigger>
        <Select.Content>
          {#each $providers as p (p.name)}
            <Select.Item value={p.name} label={p.name} />
          {/each}
        </Select.Content>
      </Select.Root>
    </div>

    <div class="space-y-2">
      <Label>IDE</Label>
      <Select.Root type="single" onValueChange={(v) => (selectedIde = v)} disabled={submitting}>
        <Select.Trigger>
          {#if selectedIde}
            <span>{IDE_OPTIONS.find((i) => i.value === selectedIde)?.label ?? selectedIde}</span>
          {:else}
            <span class="text-muted-foreground">Select an IDE</span>
          {/if}
        </Select.Trigger>
        <Select.Content>
          {#each IDE_OPTIONS as ide (ide.value)}
            <Select.Item value={ide.value} label={ide.label} />
          {/each}
        </Select.Content>
      </Select.Root>
    </div>

    <Button type="submit" disabled={submitting} class="w-full">
      {submitting ? "Creating..." : "Create Workspace"}
    </Button>
  </form>

  {#if outputLines.length > 0}
    <div class="space-y-2">
      <h2 class="text-sm font-medium">Output</h2>
      <ScrollArea class="h-64 rounded-md border bg-muted/50 p-4">
        <pre class="text-xs font-mono whitespace-pre-wrap">{outputLines.join("\n")}</pre>
      </ScrollArea>
    </div>
  {/if}
</div>
