<script lang="ts">
import { onMount } from "svelte"
import { Check, ChevronsUpDown } from "@lucide/svelte"
import { Button } from "$lib/components/ui/button/index.js"
import { Input } from "$lib/components/ui/input/index.js"
import { Label } from "$lib/components/ui/label/index.js"
import { Separator } from "$lib/components/ui/separator/index.js"
import * as Command from "$lib/components/ui/command/index.js"
import * as Popover from "$lib/components/ui/popover/index.js"
import * as Tabs from "$lib/components/ui/tabs/index.js"
import { Switch } from "$lib/components/ui/switch/index.js"
import {
  theme,
  applyTheme,
  colorScheme,
  setColorScheme,
  uiScale,
  applyUIScale,
  autoUpdate,
  setAutoUpdate,
  defaultIde,
  setDefaultIde,
  fixedIde,
  setFixedIde,
  localOptions as localOptionsStore,
  loadLocalOptions,
  saveLocalOption,
} from "$lib/stores/settings.js"
import type {
  Theme,
  ColorScheme,
  UIScale,
  LocalOptions,
} from "$lib/stores/settings.js"
import {
  devpodVersion,
  devpodUpgrade,
  devpodUpgradeDryRun,
} from "$lib/ipc/commands.js"
import { toasts } from "$lib/stores/toasts.js"

// ── Theme ───────────────────────────────────────────────────────────

const THEMES: { value: Theme; label: string }[] = [
  { value: "light", label: "Light" },
  { value: "dark", label: "Dark" },
  { value: "system", label: "System" },
]

function setTheme(value: Theme) {
  theme.set(value)
  applyTheme(value)
}

// ── Color Scheme ────────────────────────────────────────────────────

const COLOR_SCHEMES: { value: ColorScheme; label: string; swatch: string }[] = [
  { value: "default", label: "Default", swatch: "bg-foreground" },
  { value: "emerald", label: "Emerald", swatch: "bg-emerald-600" },
  { value: "purple", label: "Purple", swatch: "bg-purple-600" },
]

// ── UI Scale ────────────────────────────────────────────────────────

const UI_SCALES: { value: UIScale; label: string }[] = [
  { value: "xs", label: "Extra Small" },
  { value: "sm", label: "Small" },
  { value: "md", label: "Default" },
  { value: "lg", label: "Large" },
  { value: "xl", label: "Extra Large" },
]

function setUIScale(value: UIScale) {
  uiScale.set(value)
  applyUIScale(value)
}

// ── IDE Options ─────────────────────────────────────────────────────

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

// ── State ───────────────────────────────────────────────────────────

let activeTab = $state("general")
let cliVersion = $state<string | null>(null)
let loading = $state(true)
let saving = $state(false)
let ideComboOpen = $state(false)
let ideSearch = $state("")

let filteredIdes = $derived(
  ideSearch
    ? IDE_OPTIONS.filter((i) =>
        i.label.toLowerCase().includes(ideSearch.toLowerCase()),
      )
    : IDE_OPTIONS,
)

// Local-only options (not stored in DevPod CLI)
let local = $state<LocalOptions>({
  debugFlag: false,
  sshKeyPath: "",
  httpProxy: "",
  httpsProxy: "",
  noProxy: "",
  additionalCliFlags: "",
  additionalEnvVars: "",
  experimentalMultiDevcontainer: false,
})

// ── Version management ──────────────────────────────────────────────

let targetVersion = $state("")
let upgrading = $state(false)
let upgradeResult = $state<string | null>(null)

async function handleUpgrade() {
  if (!targetVersion) return
  const version = targetVersion.startsWith("v")
    ? targetVersion
    : `v${targetVersion}`

  // Dry-run first to validate
  try {
    const info = await devpodUpgradeDryRun(version)
    if (info.includes("already up-to-date")) {
      toasts.info(`Already running ${version}`)
      return
    }
  } catch (err) {
    toasts.error(`Invalid version: ${err}`)
    return
  }

  upgrading = true
  upgradeResult = null
  try {
    await devpodUpgrade(version)
    upgradeResult = version
    cliVersion = version.trim()
    toasts.success(`Upgraded to ${version}. Restart the app to complete.`)
  } catch (err) {
    toasts.error(`Upgrade failed: ${err}`)
  } finally {
    upgrading = false
  }
}

// ── Keyboard shortcuts ──────────────────────────────────────────────

const shortcuts = [
  { keys: "Cmd/Ctrl + K", action: "Open command palette" },
  { keys: "Cmd/Ctrl + N", action: "New workspace" },
  { keys: "Cmd/Ctrl + 1-7", action: "Navigate sections" },
  { keys: "Escape", action: "Close dialogs and palette" },
]

// ── Load / Save ─────────────────────────────────────────────────────

onMount(async () => {
  local = loadLocalOptions()
  localOptionsStore.set(local)
  loading = false
  await loadVersion()
})

async function loadVersion() {
  try {
    cliVersion = (await devpodVersion()).trim()
  } catch {
    cliVersion = null
  }
}

function saveLocal(key: keyof LocalOptions, value: string | boolean) {
  saveLocalOption(key, value)
  ;(local as unknown as Record<string, string | boolean>)[key] = value
}

function toggleLocal(key: keyof LocalOptions) {
  const current = local[key]
  saveLocal(key, !current)
}
</script>

<div class="mx-auto max-w-2xl space-y-6">
  <h1 class="text-2xl font-bold">Settings</h1>

  <Tabs.Root bind:value={activeTab}>
    <Tabs.List class="flex-wrap">
      <Tabs.Trigger value="general">General</Tabs.Trigger>
      <Tabs.Trigger value="appearance">Appearance</Tabs.Trigger>
      <Tabs.Trigger value="experimental">Experimental</Tabs.Trigger>
    </Tabs.List>

    <!-- ═══ GENERAL ═══ -->
    <Tabs.Content value="general">
      <div class="mt-4 space-y-6">
        <div class="space-y-2">
          <h2 class="text-lg font-semibold">Default IDE</h2>
          <p class="text-xs text-muted-foreground">IDE used when creating new workspaces</p>
          <Popover.Root bind:open={ideComboOpen}>
            <Popover.Trigger>
              {#snippet child({ props })}
                <Button variant="outline" class="w-full justify-between" {...props}>
                  {IDE_OPTIONS.find((i) => i.value === $defaultIde)?.label ?? "Select IDE..."}
                  <ChevronsUpDown class="ml-2 h-4 w-4 shrink-0 opacity-50" />
                </Button>
              {/snippet}
            </Popover.Trigger>
            <Popover.Content class="w-[var(--bits-popover-anchor-width)] p-0" align="start">
              <Command.Root shouldFilter={false}>
                <Command.Input placeholder="Search IDEs..." bind:value={ideSearch} />
                <Command.List>
                  <Command.Empty>No IDE found.</Command.Empty>
                  <Command.Group>
                    {#each filteredIdes as ide (ide.value)}
                      <Command.Item
                        value={ide.value}
                        onSelect={() => { setDefaultIde(ide.value); ideComboOpen = false; ideSearch = "" }}
                      >
                        <Check class="mr-2 h-4 w-4 {$defaultIde === ide.value ? 'opacity-100' : 'opacity-0'}" />
                        {ide.label}
                      </Command.Item>
                    {/each}
                  </Command.Group>
                </Command.List>
              </Command.Root>
            </Popover.Content>
          </Popover.Root>
        </div>

        <div class="flex items-center justify-between">
          <div>
            <Label>Always Use This IDE</Label>
            <p class="text-xs text-muted-foreground">Prevent IDE override when creating workspaces</p>
          </div>
          <Switch checked={$fixedIde} onCheckedChange={(v) => setFixedIde(v)} />
        </div>

        <Separator />

        <div class="flex items-center justify-between">
          <div>
            <Label>Debug Mode</Label>
            <p class="text-xs text-muted-foreground">Run all commands with --debug flag</p>
          </div>
          <Switch checked={local.debugFlag} onCheckedChange={() => toggleLocal("debugFlag")} disabled={loading || saving} />
        </div>

        <div class="space-y-2">
          <Label>SSH Key for Git Commit Signing</Label>
          <p class="text-xs text-muted-foreground">Path to SSH key for signing Git commits</p>
          <Input
            value={local.sshKeyPath}
            placeholder="~/.ssh/id_ed25519"
            oninput={(e) => (local.sshKeyPath = e.currentTarget.value)}
            onblur={() => saveLocal("sshKeyPath", local.sshKeyPath)}
            disabled={loading || saving}
          />
        </div>

        <Separator />

        <div class="space-y-4">
          <h2 class="text-lg font-semibold">Proxy Configuration</h2>
          <div class="space-y-2">
            <Label>HTTP Proxy</Label>
            <Input
              value={local.httpProxy}
              placeholder="http://proxy:8080"
              oninput={(e) => (local.httpProxy = e.currentTarget.value)}
              onblur={() => saveLocal("httpProxy", local.httpProxy)}
              disabled={loading || saving}
            />
          </div>
          <div class="space-y-2">
            <Label>HTTPS Proxy</Label>
            <Input
              value={local.httpsProxy}
              placeholder="https://proxy:8443"
              oninput={(e) => (local.httpsProxy = e.currentTarget.value)}
              onblur={() => saveLocal("httpsProxy", local.httpsProxy)}
              disabled={loading || saving}
            />
          </div>
          <div class="space-y-2">
            <Label>No Proxy</Label>
            <Input
              value={local.noProxy}
              placeholder="localhost,127.0.0.1,.internal"
              oninput={(e) => (local.noProxy = e.currentTarget.value)}
              onblur={() => saveLocal("noProxy", local.noProxy)}
              disabled={loading || saving}
            />
          </div>
        </div>

        <Separator />

        <h2 class="text-lg font-semibold">Updates</h2>

        <div class="flex items-center justify-between">
          <div>
            <Label>Automatically Keep Up to Date</Label>
            <p class="text-xs text-muted-foreground">Download and install new DevPod versions in background</p>
          </div>
          <Switch checked={$autoUpdate} onCheckedChange={(v) => setAutoUpdate(v)} />
        </div>

        <div class="space-y-2">
          <Label>Current Version</Label>
          {#if cliVersion}
            <p class="text-sm">DevPod CLI: <span class="font-mono">{cliVersion}</span></p>
          {:else}
            <p class="text-sm text-muted-foreground">Version information not available</p>
          {/if}
        </div>

        <div class="space-y-3">
          <Label>Switch Version</Label>
          <p class="text-xs text-muted-foreground">Install a specific DevPod CLI version. Useful for downgrading if a newer version introduced issues.</p>
          <div class="flex gap-2">
            <Input
              value={targetVersion}
              placeholder="e.g. v0.20.0"
              oninput={(e) => (targetVersion = e.currentTarget.value)}
              onkeydown={(e) => { if (e.key === "Enter") handleUpgrade() }}
              disabled={upgrading}
              class="max-w-48 font-mono"
            />
            <Button
              onclick={handleUpgrade}
              disabled={upgrading || !targetVersion}
              variant="outline"
            >
              {upgrading ? "Installing..." : "Install"}
            </Button>
          </div>
          {#if upgradeResult}
            <div class="rounded-md border border-green-600/30 bg-green-600/10 p-3">
              <p class="text-sm text-green-700 dark:text-green-400">
                Switched to {upgradeResult}. Restart the application to use the new version.
              </p>
            </div>
          {/if}
        </div>

        <Separator />

        <div class="space-y-2">
          <h2 class="text-lg font-semibold">Keyboard Shortcuts</h2>
          <div class="rounded-md border divide-y">
            {#each shortcuts as shortcut}
              <div class="flex items-center justify-between px-4 py-2 text-sm">
                <span class="text-muted-foreground">{shortcut.action}</span>
                <kbd class="rounded border bg-muted px-2 py-0.5 text-xs font-mono">{shortcut.keys}</kbd>
              </div>
            {/each}
          </div>
        </div>

        <Separator />

        <div class="space-y-1 text-sm text-muted-foreground">
          <p>DevPod Desktop</p>
          <p>Built with Tauri v2 + SvelteKit + shadcn-svelte</p>
        </div>
      </div>
    </Tabs.Content>

    <!-- ═══ APPEARANCE ═══ -->
    <Tabs.Content value="appearance">
      <div class="mt-4 space-y-6">
        <div class="space-y-2">
          <h2 class="text-lg font-semibold">Theme</h2>
          <div class="flex gap-2">
            {#each THEMES as t (t.value)}
              <Button
                variant={$theme === t.value ? "default" : "outline"}
                onclick={() => setTheme(t.value)}
              >
                {t.label}
              </Button>
            {/each}
          </div>
        </div>

        <Separator />

        <div class="space-y-2">
          <h2 class="text-lg font-semibold">Color Scheme</h2>
          <div class="flex gap-2">
            {#each COLOR_SCHEMES as c (c.value)}
              <Button
                variant={$colorScheme === c.value ? "default" : "outline"}
                onclick={() => setColorScheme(c.value)}
                class="gap-2"
              >
                <span class="h-3 w-3 rounded-full {c.swatch}"></span>
                {c.label}
              </Button>
            {/each}
          </div>
        </div>

        <Separator />

        <div class="space-y-2">
          <h2 class="text-lg font-semibold">UI Scale</h2>
          <p class="text-xs text-muted-foreground">Adjust the overall size of text and interface elements</p>
          <div class="flex gap-2">
            {#each UI_SCALES as s (s.value)}
              <Button
                variant={$uiScale === s.value ? "default" : "outline"}
                onclick={() => setUIScale(s.value)}
              >
                {s.label}
              </Button>
            {/each}
          </div>
        </div>

      </div>
    </Tabs.Content>

    <!-- ═══ EXPERIMENTAL ═══ -->
    <Tabs.Content value="experimental">
      <div class="mt-4 space-y-6">
        <div class="rounded-md border border-yellow-500/30 bg-yellow-500/5 p-3">
          <p class="text-sm text-yellow-600 dark:text-yellow-400">
            Experimental features may be unstable. Use at your own risk.
          </p>
        </div>

        <div class="flex items-center justify-between">
          <div>
            <Label>Multiple Devcontainer Detection</Label>
            <p class="text-xs text-muted-foreground">Check for multiple devcontainers when creating workspaces. May take longer for larger repos.</p>
          </div>
          <Switch checked={local.experimentalMultiDevcontainer} onCheckedChange={() => toggleLocal("experimentalMultiDevcontainer")} disabled={loading || saving} />
        </div>

        <Separator />

        <div class="space-y-2">
          <Label>Additional CLI Flags</Label>
          <p class="text-xs text-muted-foreground">Append custom flags to all DevPod CLI commands</p>
          <Input
            value={local.additionalCliFlags}
            placeholder="--flag1 --flag2=value"
            oninput={(e) => (local.additionalCliFlags = e.currentTarget.value)}
            onblur={() => saveLocal("additionalCliFlags", local.additionalCliFlags)}
            disabled={loading || saving}
          />
        </div>

        <div class="space-y-2">
          <Label>Additional Environment Variables</Label>
          <p class="text-xs text-muted-foreground">Comma-separated environment variables passed to DevPod commands</p>
          <Input
            value={local.additionalEnvVars}
            placeholder="FOO=bar,BAZ=false"
            oninput={(e) => (local.additionalEnvVars = e.currentTarget.value)}
            onblur={() => saveLocal("additionalEnvVars", local.additionalEnvVars)}
            disabled={loading || saving}
          />
        </div>
      </div>
    </Tabs.Content>
  </Tabs.Root>
</div>
