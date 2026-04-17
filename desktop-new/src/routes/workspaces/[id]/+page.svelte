<script lang="ts">
import { page } from "$app/stores"
import { goto } from "$app/navigation"
import { onMount, onDestroy } from "svelte"
import { Button } from "$lib/components/ui/button/index.js"
import { badgeVariants } from "$lib/components/ui/badge/index.js"
import { Separator } from "$lib/components/ui/separator/index.js"
import * as Select from "$lib/components/ui/select/index.js"
import * as Table from "$lib/components/ui/table/index.js"
import * as Tabs from "$lib/components/ui/tabs/index.js"
import { ScrollArea } from "$lib/components/ui/scroll-area/index.js"
import ConfirmDialog from "$lib/components/layout/ConfirmDialog.svelte"
import TerminalComponent from "$lib/components/terminal/Terminal.svelte"
import { workspaces } from "$lib/stores/workspaces.js"
import { addTerminal, removeTerminal } from "$lib/stores/terminals.js"
import { terminalCreateSsh, terminalClose } from "$lib/ipc/terminal.js"
import {
  workspaceUp,
  workspaceStop,
  workspaceRebuild,
  workspaceReset,
  workspaceDelete,
  workspaceLogsList,
  workspaceLogRead,
  auditByResource,
} from "$lib/ipc/commands.js"
import { onCommandProgress } from "$lib/ipc/events.js"
import { toasts } from "$lib/stores/toasts.js"
import type { AuditEntry, LogEntry } from "$lib/types/index.js"
import type { UnlistenFn } from "@tauri-apps/api/event"
import { formatTimestamp } from "$lib/utils/time.js"
import { parseLogLine, stripAnsi } from "$lib/utils/log-parser.js"

const IDE_OPTIONS = [
  { value: "none", label: "None" },
  { value: "vscode", label: "VS Code" },
  { value: "openvscode", label: "VS Code Browser" },
  { value: "cursor", label: "Cursor" },
  { value: "zed", label: "Zed" },
  { value: "codium", label: "VSCodium" },
  { value: "windsurf", label: "Windsurf Editor" },
  { value: "antigravity", label: "Google Antigravity" },
  { value: "bob", label: "IBM Bob" },
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
  { value: "jupyternotebook", label: "Jupyter Notebook" },
  { value: "vscode-insiders", label: "VS Code Insiders" },
  { value: "positron", label: "Positron" },
  { value: "rstudio", label: "RStudio Server" },
]

let id = $derived($page.params.id as string)
let workspace = $derived($workspaces.find((ws) => ws.id === id))

let isRunning = $derived(workspace?.status?.toLowerCase() === "running")
let isStopped = $derived(
  !workspace?.status ||
    workspace.status.toLowerCase() === "stopped" ||
    workspace.status.toLowerCase() === "notfound",
)
let isBusy = $derived(workspace?.status?.toLowerCase() === "busy")

function statusBadgeVariant(): "default" | "secondary" | "outline" {
  if (isRunning) return "default"
  if (isBusy) return "secondary"
  return "outline"
}

let activeTab = $state("overview")
let outputLines = $state<string[]>([])
let parsedLines = $derived(outputLines.map(parseLogLine))
let commandId = $state<string | null>(null)
let operationLabel = $state("")
let operationRunning = $state(false)
let unlisten: UnlistenFn | null = null
let tableEndEl = $state<HTMLDivElement | null>(null)

let logEntries = $state<LogEntry[]>([])
let selectedLog = $state<string | null>(null)
let logContent = $state<string>("")
let logsLoading = $state(false)

let auditEntries = $state<AuditEntry[]>([])
let auditLoading = $state(false)
let confirmDeleteOpen = $state(false)
let deleting = $state(false)

let sshSessionId = $state<string | null>(null)
let connecting = $state(false)

function scrollToBottom() {
  if (tableEndEl) {
    tableEndEl.scrollIntoView({ block: "end", behavior: "smooth" })
  }
}

async function copyToClipboard(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    toasts.success("Copied to clipboard")
  } catch {
    toasts.error("Failed to copy to clipboard")
  }
}

onMount(async () => {
  try {
    unlisten = await onCommandProgress((progress) => {
      if (commandId && progress.commandId === commandId) {
        if (progress.message) {
          outputLines = [...outputLines, progress.message]
          requestAnimationFrame(scrollToBottom)
        }
        if (progress.done) {
          operationRunning = false
          const success = stripAnsi(progress.message).includes("Exit code: 0")
          if (success) {
            toasts.success(`${operationLabel} ${id} succeeded`)
          } else {
            toasts.error(
              `${operationLabel} ${id} failed. Check output for details.`,
            )
          }
          if (operationLabel === "Delete" && success) {
            goto("/workspaces")
            return
          }
          loadAudit()
          loadLogs()
        }
      }
    })
  } catch {
    // Event listener setup failed
  }

  loadLogs()
  loadAudit()
})

onDestroy(() => {
  unlisten?.()
  // Clean up SSH session if navigating away
  if (sshSessionId) {
    terminalClose(sshSessionId).catch(() => {})
    removeTerminal(sshSessionId)
  }
})

async function loadLogs() {
  logsLoading = true
  try {
    logEntries = await workspaceLogsList(id)
  } catch {
    logEntries = []
  } finally {
    logsLoading = false
  }
}

async function loadAudit() {
  auditLoading = true
  try {
    auditEntries = await auditByResource("workspace", id)
  } catch {
    auditEntries = []
  } finally {
    auditLoading = false
  }
}

async function viewLog(entry: LogEntry) {
  selectedLog = entry.filename
  try {
    logContent = await workspaceLogRead(id, entry.filename)
  } catch {
    logContent = "Failed to load log content."
  }
}

async function handleConnect() {
  connecting = true
  try {
    const sessionId = await terminalCreateSsh(id, 80, 24)
    sshSessionId = sessionId
    addTerminal({
      id: sessionId,
      label: `SSH: ${id}`,
      type: "ssh",
      workspaceId: id,
    })
    activeTab = "terminal"
    toasts.success(`Connected to ${id}`)
  } catch (err) {
    toasts.error(`Failed to connect: ${err}`)
  } finally {
    connecting = false
  }
}

async function handleDisconnect() {
  if (!sshSessionId) return
  try {
    await terminalClose(sshSessionId)
  } catch {
    // session may already be gone
  }
  removeTerminal(sshSessionId)
  sshSessionId = null
}

function handleSshExit() {
  if (sshSessionId) {
    removeTerminal(sshSessionId)
    sshSessionId = null
  }
}

function startStreamingOp(label: string) {
  operationLabel = label
  operationRunning = true
  outputLines = []
  activeTab = "output"
}

async function handleStart() {
  startStreamingOp("Start")
  try {
    commandId = await workspaceUp({ source: id })
  } catch (err) {
    operationRunning = false
    toasts.error(`Failed to start: ${err}`)
  }
}

async function handleOpenIde() {
  const ide = workspace?.ide?.name || undefined
  startStreamingOp("Open IDE")
  try {
    commandId = await workspaceUp({ source: id, ide })
  } catch (err) {
    operationRunning = false
    toasts.error(`Failed to open IDE: ${err}`)
  }
}

async function handleStop() {
  startStreamingOp("Stop")
  try {
    commandId = await workspaceStop(id)
  } catch (err) {
    operationRunning = false
    toasts.error(`Failed to stop: ${err}`)
  }
}

async function handleRebuild() {
  startStreamingOp("Rebuild")
  try {
    commandId = await workspaceRebuild(id)
  } catch (err) {
    operationRunning = false
    toasts.error(`Failed to rebuild: ${err}`)
  }
}

async function handleReset() {
  startStreamingOp("Reset")
  try {
    commandId = await workspaceReset(id)
  } catch (err) {
    operationRunning = false
    toasts.error(`Failed to reset: ${err}`)
  }
}

async function handleDelete() {
  confirmDeleteOpen = false
  startStreamingOp("Delete")
  deleting = true
  try {
    commandId = await workspaceDelete(id)
  } catch (err) {
    operationRunning = false
    deleting = false
    toasts.error(`Failed to delete: ${err}`)
  }
}
</script>

<div class="space-y-6">
  <div class="flex items-center gap-4">
    <Button variant="ghost" size="sm" onclick={() => goto("/workspaces")}>
      &larr; Back
    </Button>
    <h1 class="text-2xl font-bold">{id}</h1>
    {#if workspace?.provider?.name}
      <span class={badgeVariants({ variant: "secondary" })}>{workspace.provider.name}</span>
    {/if}
    {#if workspace?.status}
      <span class={badgeVariants({ variant: statusBadgeVariant() })}>{workspace.status}</span>
    {/if}
  </div>

  <div class="flex items-center gap-2">
    {#if isRunning}
      <Button size="sm" onclick={handleOpenIde} disabled={operationRunning}>
        Open IDE
      </Button>
    {/if}
    {#if sshSessionId}
      <Button variant="outline" size="sm" onclick={handleDisconnect}>Disconnect</Button>
    {:else if isRunning}
      <Button variant="outline" size="sm" onclick={handleConnect} disabled={connecting}>
        {connecting ? "Connecting..." : "SSH Terminal"}
      </Button>
    {/if}
    {#if isStopped}
      <Button size="sm" onclick={handleStart} disabled={operationRunning || connecting}>
        {operationRunning ? "Starting..." : "Start"}
      </Button>
    {/if}
    {#if isRunning || isBusy}
      <Button variant="outline" size="sm" onclick={handleStop} disabled={operationRunning}>Stop</Button>
    {/if}
    <Button variant="outline" size="sm" onclick={handleRebuild} disabled={operationRunning}>Rebuild</Button>
    <Button variant="outline" size="sm" onclick={handleReset} disabled={operationRunning}>Reset</Button>
    <Button variant="destructive" size="sm" onclick={() => (confirmDeleteOpen = true)} disabled={operationRunning}>Delete</Button>
    {#if operationRunning}
      <span class="text-xs text-muted-foreground animate-pulse">{operationLabel}ing workspace...</span>
    {/if}
  </div>

  <Separator />

  {#if !workspace}
    <p class="text-muted-foreground">Workspace not found.</p>
  {:else}
    <Tabs.Root bind:value={activeTab}>
      <Tabs.List>
        <Tabs.Trigger value="overview">Overview</Tabs.Trigger>
        <Tabs.Trigger value="output">Live Output</Tabs.Trigger>
        <Tabs.Trigger value="terminal">Terminal</Tabs.Trigger>
        <Tabs.Trigger value="logs">Logs</Tabs.Trigger>
        <Tabs.Trigger value="activity">Activity</Tabs.Trigger>
      </Tabs.List>

      <Tabs.Content value="overview">
        <div class="mt-4 grid grid-cols-2 gap-4 text-sm">
          <div class="text-muted-foreground">ID</div>
          <div>{workspace.id}</div>

          <div class="text-muted-foreground">UID</div>
          <div>{workspace.uid ?? "N/A"}</div>

          <div class="text-muted-foreground">Provider</div>
          <div>{workspace.provider?.name ?? "N/A"}</div>

          <div class="text-muted-foreground">Machine</div>
          <div>{workspace.machine?.id ?? "N/A"}</div>

          <div class="text-muted-foreground">IDE</div>
          <div>
            <Select.Root
              type="single"
              value={workspace.ide?.name ?? "none"}
              onValueChange={async (newIde) => {
                try {
                  startStreamingOp("Change IDE")
                  commandId = await workspaceUp({ source: id, ide: newIde })
                } catch (err) {
                  operationRunning = false
                  toasts.error(`Failed to change IDE: ${err}`)
                }
              }}
            >
              <Select.Trigger class="h-8 w-48">
                <span>{IDE_OPTIONS.find((i) => i.value === (workspace.ide?.name ?? "none"))?.label ?? "None"}</span>
              </Select.Trigger>
              <Select.Content class="max-h-80">
                {#each IDE_OPTIONS as ide (ide.value)}
                  <Select.Item value={ide.value} label={ide.label} />
                {/each}
              </Select.Content>
            </Select.Root>
          </div>

          <div class="text-muted-foreground">Source</div>
          <div class="truncate">
            {workspace.source?.gitRepository
              ?? workspace.source?.localFolder
              ?? workspace.source?.image
              ?? "N/A"}
          </div>

          {#if workspace.source?.gitBranch}
            <div class="text-muted-foreground">Branch</div>
            <div>{workspace.source.gitBranch}</div>
          {/if}

          <div class="text-muted-foreground">Status</div>
          <div>{workspace.status ?? "Unknown"}</div>

          <div class="text-muted-foreground">Created</div>
          <div>{workspace.creationTimestamp ? formatTimestamp(workspace.creationTimestamp) : "N/A"}</div>

          <div class="text-muted-foreground">Last Used</div>
          <div>{workspace.lastUsedTimestamp ? formatTimestamp(workspace.lastUsedTimestamp) : "N/A"}</div>

          <div class="text-muted-foreground">Context</div>
          <div>{workspace.context ?? "N/A"}</div>
        </div>
      </Tabs.Content>

      <Tabs.Content value="output">
        <div class="mt-4 flex flex-col" style="height: calc(100vh - 20rem);">
          {#if outputLines.length > 0}
            <div class="flex justify-end gap-2 shrink-0 mb-2">
              <Button variant="outline" size="sm" onclick={() => copyToClipboard(outputLines.map(stripAnsi).join("\n"))}>
                Copy Output
              </Button>
              <Button variant="outline" size="sm" onclick={() => { outputLines = [] }}>
                Clear
              </Button>
            </div>
          {/if}
          <ScrollArea class="min-h-0 flex-1 rounded-md border">
            {#if outputLines.length === 0}
              <div class="flex items-center justify-center h-full p-4">
                <p class="text-sm text-muted-foreground">
                  {operationRunning ? "Waiting for output..." : "No output yet. Run an operation to see live output."}
                </p>
              </div>
            {:else}
              <Table.Root>
                <Table.Header>
                  <Table.Row>
                    <Table.Head class="w-20">Time</Table.Head>
                    <Table.Head class="w-16">Level</Table.Head>
                    <Table.Head>Message</Table.Head>
                    <Table.Head class="w-40 text-right">Source</Table.Head>
                  </Table.Row>
                </Table.Header>
                <Table.Body>
                  {#each parsedLines as line, i (i)}
                    <Table.Row class={line.level === "fatal" || line.level === "error" ? "bg-destructive/5" : line.level === "warn" ? "bg-amber-500/5" : ""}>
                      <Table.Cell class="font-mono text-xs text-muted-foreground">{line.time}</Table.Cell>
                      <Table.Cell>
                        {#if line.level}
                          <span class={badgeVariants({ variant: line.level === "fatal" || line.level === "error" ? "destructive" : line.level === "warn" ? "outline" : "secondary" })}>
                            {line.level}
                          </span>
                        {/if}
                      </Table.Cell>
                      <Table.Cell class="text-sm">{line.message}</Table.Cell>
                      <Table.Cell class="font-mono text-xs text-muted-foreground text-right">{line.source}</Table.Cell>
                    </Table.Row>
                  {/each}
                </Table.Body>
              </Table.Root>
              <div bind:this={tableEndEl}></div>
            {/if}
          </ScrollArea>
        </div>
      </Tabs.Content>

      <Tabs.Content value="terminal">
        <div class="mt-4 flex flex-col" style="height: calc(100vh - 20rem);">
          {#if sshSessionId}
            <div class="min-h-0 flex-1 rounded-md border overflow-hidden">
              <TerminalComponent sessionId={sshSessionId} onExit={handleSshExit} />
            </div>
            <div class="mt-2 flex justify-end shrink-0">
              <Button variant="outline" size="sm" onclick={handleDisconnect}>Disconnect</Button>
            </div>
          {:else}
            <div class="flex min-h-0 flex-1 items-center justify-center rounded-md border bg-muted/50">
              <div class="text-center">
                <p class="text-muted-foreground">No active terminal session.</p>
                <Button size="sm" class="mt-3" onclick={handleConnect} disabled={connecting}>
                  {connecting ? "Connecting..." : "Connect to workspace"}
                </Button>
              </div>
            </div>
          {/if}
        </div>
      </Tabs.Content>

      <Tabs.Content value="logs">
        <div class="mt-4 space-y-4">
          <div class="flex justify-end gap-2">
            <Button variant="outline" size="sm" onclick={loadLogs} disabled={logsLoading}>
              {logsLoading ? "Loading..." : "Refresh"}
            </Button>
            {#if selectedLog && logContent}
              <Button variant="outline" size="sm" onclick={() => copyToClipboard(logContent)}>
                Copy Log
              </Button>
            {/if}
          </div>
          {#if logsLoading}
            <p class="text-sm text-muted-foreground">Loading logs...</p>
          {:else if logEntries.length === 0}
            <p class="text-sm text-muted-foreground">
              No logs found for this workspace.
            </p>
          {:else}
            <div class="flex gap-4">
              <div class="w-64 space-y-1">
                {#each logEntries as entry}
                  <button
                    class="w-full rounded px-3 py-2 text-left text-sm hover:bg-muted {selectedLog === entry.filename ? 'bg-muted font-medium' : ''}"
                    onclick={() => viewLog(entry)}
                  >
                    <div class="truncate">{entry.filename}</div>
                    <div class="text-xs text-muted-foreground">
                      {Math.round(entry.sizeBytes / 1024)}KB
                    </div>
                  </button>
                {/each}
              </div>
              <ScrollArea class="h-96 flex-1 rounded-md border bg-muted/50 p-4">
                {#if selectedLog}
                  <pre class="text-xs font-mono whitespace-pre-wrap">{logContent}</pre>
                {:else}
                  <p class="text-sm text-muted-foreground">Select a log file to view.</p>
                {/if}
              </ScrollArea>
            </div>
          {/if}
        </div>
      </Tabs.Content>

      <Tabs.Content value="activity">
        <div class="mt-4 space-y-4">
          {#if auditLoading}
            <p class="text-sm text-muted-foreground">Loading activity...</p>
          {:else if auditEntries.length === 0}
            <p class="text-sm text-muted-foreground">
              No activity recorded for this workspace.
            </p>
          {:else}
            <div class="divide-y rounded-md border">
              {#each auditEntries as entry}
                <div class="flex items-center gap-3 px-4 py-3">
                  <span
                    class={badgeVariants({
                      variant: entry.success ? "default" : "destructive",
                    })}
                  >
                    {entry.action}
                  </span>
                  <div class="min-w-0 flex-1">
                    {#if entry.details}
                      <span class="text-sm text-muted-foreground">{entry.details}</span>
                    {/if}
                  </div>
                  <span class="shrink-0 text-xs text-muted-foreground">
                    {formatTimestamp(entry.timestamp)}
                  </span>
                </div>
              {/each}
            </div>
          {/if}
        </div>
      </Tabs.Content>
    </Tabs.Root>
  {/if}
</div>

<ConfirmDialog
  bind:open={confirmDeleteOpen}
  title="Delete workspace"
  description="This will permanently delete workspace '{id}' and all associated data. This action cannot be undone."
  confirmLabel="Delete"
  loading={deleting}
  onconfirm={handleDelete}
/>
