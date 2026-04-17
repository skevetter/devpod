<script lang="ts">
import { onMount } from "svelte"
import { Button } from "$lib/components/ui/button/index.js"
import { Input } from "$lib/components/ui/input/index.js"
import { Label } from "$lib/components/ui/label/index.js"
import { Separator } from "$lib/components/ui/separator/index.js"
import * as Select from "$lib/components/ui/select/index.js"
import * as Tabs from "$lib/components/ui/tabs/index.js"
import { Switch } from "$lib/components/ui/switch/index.js"
import {
  theme,
  applyTheme,
  colorScheme,
  setColorScheme,
  fontSize,
  applyFontSize,
  zoomLevel,
  applyZoom,
  sidebarPosition,
  setSidebarPosition,
  autoUpdate,
  setAutoUpdate,
  defaultIde,
  setDefaultIde,
  fixedIde,
  setFixedIde,
  contextOptions as contextOptionsStore,
  parseContextOptions,
  CONTEXT_OPTION_KEYS,
} from "$lib/stores/settings.js"
import type {
  Theme,
  ColorScheme,
  FontSize,
  ZoomLevel,
  SidebarPosition,
  ContextOptions,
} from "$lib/stores/settings.js"
import {
  contextOptions as fetchContextOptions,
  contextSetOptions,
  devpodVersion,
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

// ── Font Size ───────────────────────────────────────────────────────

const FONT_SIZES: { value: FontSize; label: string }[] = [
  { value: "small", label: "Small" },
  { value: "medium", label: "Medium" },
  { value: "large", label: "Large" },
]

function setFontSize(value: FontSize) {
  fontSize.set(value)
  applyFontSize(value)
}

// ── Zoom ────────────────────────────────────────────────────────────

const ZOOM_LEVELS: { value: ZoomLevel; label: string }[] = [
  { value: "sm", label: "Small" },
  { value: "md", label: "Regular" },
  { value: "lg", label: "Large" },
  { value: "xl", label: "Extra Large" },
]

function setZoom(value: ZoomLevel) {
  zoomLevel.set(value)
  applyZoom(value)
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

// Context options (mutable local copy)
let opts = $state<ContextOptions>({
  debugFlag: false,
  telemetry: true,
  agentUrl: "",
  dotfilesUrl: "",
  sshKeyPath: "",
  httpProxy: "",
  httpsProxy: "",
  noProxy: "",
  dockerCredentialForwarding: false,
  gitCredentialForwarding: false,
  gitSshSignatureForwarding: false,
  sshAgentForwarding: false,
  sshAddPrivateKeys: false,
  sshStrictHostKeyChecking: false,
  gpgAgentForwarding: false,
  additionalCliFlags: "",
  additionalEnvVars: "",
  experimentalMultiDevcontainer: false,
})

// ── Keyboard shortcuts ──────────────────────────────────────────────

const shortcuts = [
  { keys: "Cmd/Ctrl + K", action: "Open command palette" },
  { keys: "Cmd/Ctrl + N", action: "New workspace" },
  { keys: "Cmd/Ctrl + 1-7", action: "Navigate sections" },
  { keys: "Escape", action: "Close dialogs and palette" },
]

// ── Load / Save ─────────────────────────────────────────────────────

onMount(async () => {
  await Promise.all([loadContextOptions(), loadVersion()])
})

async function loadVersion() {
  try {
    cliVersion = (await devpodVersion()).trim()
  } catch {
    cliVersion = null
  }
}

async function loadContextOptions() {
  loading = true
  try {
    const raw = await fetchContextOptions()
    const parsed = parseContextOptions(raw)
    opts = parsed
    contextOptionsStore.set(parsed)
  } catch {
    // Keep defaults
  } finally {
    loading = false
  }
}

async function saveContextOption(
  key: keyof ContextOptions,
  value: string | boolean,
) {
  saving = true
  const cliKey = CONTEXT_OPTION_KEYS[key]
  const strValue = String(value)
  try {
    await contextSetOptions([`${cliKey}=${strValue}`])
    ;(opts as unknown as Record<string, string | boolean>)[key] = value
    contextOptionsStore.set({ ...opts })
    toasts.success("Setting saved")
  } catch (err) {
    toasts.error(`Failed to save: ${err}`)
  } finally {
    saving = false
  }
}

function toggleContextOption(key: keyof ContextOptions) {
  const current = opts[key]
  saveContextOption(key, !current)
}
</script>

<div class="mx-auto max-w-2xl space-y-6">
  <h1 class="text-2xl font-bold">Settings</h1>

  <Tabs.Root bind:value={activeTab}>
    <Tabs.List class="flex-wrap">
      <Tabs.Trigger value="general">General</Tabs.Trigger>
      <Tabs.Trigger value="customization">Customization</Tabs.Trigger>
      <Tabs.Trigger value="appearance">Appearance</Tabs.Trigger>
      <Tabs.Trigger value="updates">Updates</Tabs.Trigger>
      <Tabs.Trigger value="experimental">Experimental</Tabs.Trigger>
    </Tabs.List>

    <!-- ═══ GENERAL ═══ -->
    <Tabs.Content value="general">
      <div class="mt-4 space-y-6">
        <div class="space-y-2">
          <h2 class="text-lg font-semibold">CLI</h2>
          {#if cliVersion}
            <p class="text-sm text-muted-foreground">DevPod CLI: <span class="font-mono">{cliVersion}</span></p>
          {:else}
            <p class="text-sm text-muted-foreground">CLI version not available</p>
          {/if}
        </div>

        <Separator />

        <div class="flex items-center justify-between">
          <div>
            <Label>Debug Mode</Label>
            <p class="text-xs text-muted-foreground">Run all commands with --debug flag</p>
          </div>
          <Switch checked={opts.debugFlag} onCheckedChange={() => toggleContextOption("debugFlag")} disabled={loading || saving} />
        </div>

        <Separator />

        <div class="space-y-2">
          <Label>Agent URL</Label>
          <p class="text-xs text-muted-foreground">Custom agent endpoint URL</p>
          <Input
            value={opts.agentUrl}
            placeholder="Leave empty for default"
            oninput={(e) => (opts.agentUrl = e.currentTarget.value)}
            onblur={() => saveContextOption("agentUrl", opts.agentUrl)}
            disabled={loading || saving}
          />
        </div>

        <Separator />

        <div class="space-y-4">
          <h2 class="text-lg font-semibold">Proxy Configuration</h2>
          <div class="space-y-2">
            <Label>HTTP Proxy</Label>
            <Input
              value={opts.httpProxy}
              placeholder="http://proxy:8080"
              oninput={(e) => (opts.httpProxy = e.currentTarget.value)}
              onblur={() => saveContextOption("httpProxy", opts.httpProxy)}
              disabled={loading || saving}
            />
          </div>
          <div class="space-y-2">
            <Label>HTTPS Proxy</Label>
            <Input
              value={opts.httpsProxy}
              placeholder="https://proxy:8443"
              oninput={(e) => (opts.httpsProxy = e.currentTarget.value)}
              onblur={() => saveContextOption("httpsProxy", opts.httpsProxy)}
              disabled={loading || saving}
            />
          </div>
          <div class="space-y-2">
            <Label>No Proxy</Label>
            <Input
              value={opts.noProxy}
              placeholder="localhost,127.0.0.1,.internal"
              oninput={(e) => (opts.noProxy = e.currentTarget.value)}
              onblur={() => saveContextOption("noProxy", opts.noProxy)}
              disabled={loading || saving}
            />
          </div>
        </div>

        <Separator />

        <div class="flex items-center justify-between">
          <div>
            <Label>Telemetry</Label>
            <p class="text-xs text-muted-foreground">Send anonymous usage data</p>
          </div>
          <Switch checked={opts.telemetry} onCheckedChange={() => toggleContextOption("telemetry")} disabled={loading || saving} />
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

    <!-- ═══ CUSTOMIZATION ═══ -->
    <Tabs.Content value="customization">
      <div class="mt-4 space-y-6">
        <div class="space-y-2">
          <Label>Default IDE</Label>
          <p class="text-xs text-muted-foreground">IDE used when creating new workspaces</p>
          <Select.Root type="single" value={$defaultIde} onValueChange={(v) => { if (v) setDefaultIde(v) }}>
            <Select.Trigger class="w-full">
              <span>{IDE_OPTIONS.find((i) => i.value === $defaultIde)?.label ?? $defaultIde}</span>
            </Select.Trigger>
            <Select.Content class="max-h-80 w-[var(--bits-select-trigger-width)]">
              {#each IDE_OPTIONS as ide (ide.value)}
                <Select.Item value={ide.value} label={ide.label} />
              {/each}
            </Select.Content>
          </Select.Root>
        </div>

        <div class="flex items-center justify-between">
          <div>
            <Label>Always Use This IDE</Label>
            <p class="text-xs text-muted-foreground">Prevent IDE override when creating workspaces</p>
          </div>
          <Switch checked={$fixedIde} onCheckedChange={(v) => setFixedIde(v)} />
        </div>

        <Separator />

        <div class="space-y-2">
          <Label>Dotfiles Repository</Label>
          <p class="text-xs text-muted-foreground">Git repository URL for dotfiles to apply in workspaces</p>
          <Input
            value={opts.dotfilesUrl}
            placeholder="https://github.com/user/dotfiles"
            oninput={(e) => (opts.dotfilesUrl = e.currentTarget.value)}
            onblur={() => saveContextOption("dotfilesUrl", opts.dotfilesUrl)}
            disabled={loading || saving}
          />
        </div>

        <div class="space-y-2">
          <Label>SSH Key for Git Commit Signing</Label>
          <p class="text-xs text-muted-foreground">Path to SSH key for signing Git commits</p>
          <Input
            value={opts.sshKeyPath}
            placeholder="~/.ssh/id_ed25519"
            oninput={(e) => (opts.sshKeyPath = e.currentTarget.value)}
            onblur={() => saveContextOption("sshKeyPath", opts.sshKeyPath)}
            disabled={loading || saving}
          />
        </div>

        <Separator />

        <h2 class="text-lg font-semibold">Forwarding</h2>

        <div class="space-y-4">
          <div class="flex items-center justify-between">
            <div>
              <Label>Docker Credentials</Label>
              <p class="text-xs text-muted-foreground">Forward Docker credentials to workspaces</p>
            </div>
            <Switch checked={opts.dockerCredentialForwarding} onCheckedChange={() => toggleContextOption("dockerCredentialForwarding")} disabled={loading || saving} />
          </div>

          <div class="flex items-center justify-between">
            <div>
              <Label>Git Credentials</Label>
              <p class="text-xs text-muted-foreground">Forward Git credential helper to workspaces</p>
            </div>
            <Switch checked={opts.gitCredentialForwarding} onCheckedChange={() => toggleContextOption("gitCredentialForwarding")} disabled={loading || saving} />
          </div>

          <div class="flex items-center justify-between">
            <div>
              <Label>Git SSH Signature</Label>
              <p class="text-xs text-muted-foreground">Forward Git SSH signature to workspaces</p>
            </div>
            <Switch checked={opts.gitSshSignatureForwarding} onCheckedChange={() => toggleContextOption("gitSshSignatureForwarding")} disabled={loading || saving} />
          </div>

          <div class="flex items-center justify-between">
            <div>
              <Label>SSH Agent</Label>
              <p class="text-xs text-muted-foreground">Forward SSH agent to workspaces</p>
            </div>
            <Switch checked={opts.sshAgentForwarding} onCheckedChange={() => toggleContextOption("sshAgentForwarding")} disabled={loading || saving} />
          </div>

          <div class="flex items-center justify-between">
            <div>
              <Label>SSH Add Private Keys</Label>
              <p class="text-xs text-muted-foreground">Automatically add private SSH keys to agent</p>
            </div>
            <Switch checked={opts.sshAddPrivateKeys} onCheckedChange={() => toggleContextOption("sshAddPrivateKeys")} disabled={loading || saving} />
          </div>

          <div class="flex items-center justify-between">
            <div>
              <Label>SSH Strict Host Key Checking</Label>
              <p class="text-xs text-muted-foreground">Enable strict SSH host key verification</p>
            </div>
            <Switch checked={opts.sshStrictHostKeyChecking} onCheckedChange={() => toggleContextOption("sshStrictHostKeyChecking")} disabled={loading || saving} />
          </div>

          <div class="flex items-center justify-between">
            <div>
              <Label>GPG Agent</Label>
              <p class="text-xs text-muted-foreground">Forward GPG agent to workspaces</p>
            </div>
            <Switch checked={opts.gpgAgentForwarding} onCheckedChange={() => toggleContextOption("gpgAgentForwarding")} disabled={loading || saving} />
          </div>
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
          <h2 class="text-lg font-semibold">Font Size</h2>
          <div class="flex gap-2">
            {#each FONT_SIZES as f (f.value)}
              <Button
                variant={$fontSize === f.value ? "default" : "outline"}
                onclick={() => setFontSize(f.value)}
              >
                {f.label}
              </Button>
            {/each}
          </div>
        </div>

        <Separator />

        <div class="space-y-2">
          <h2 class="text-lg font-semibold">Zoom Level</h2>
          <div class="flex gap-2">
            {#each ZOOM_LEVELS as z (z.value)}
              <Button
                variant={$zoomLevel === z.value ? "default" : "outline"}
                onclick={() => setZoom(z.value)}
              >
                {z.label}
              </Button>
            {/each}
          </div>
        </div>

        <Separator />

        <div class="space-y-2">
          <h2 class="text-lg font-semibold">Sidebar Position</h2>
          <div class="flex gap-2">
            <Button
              variant={$sidebarPosition === "left" ? "default" : "outline"}
              onclick={() => setSidebarPosition("left")}
            >
              Left
            </Button>
            <Button
              variant={$sidebarPosition === "right" ? "default" : "outline"}
              onclick={() => setSidebarPosition("right")}
            >
              Right
            </Button>
          </div>
        </div>
      </div>
    </Tabs.Content>

    <!-- ═══ UPDATES ═══ -->
    <Tabs.Content value="updates">
      <div class="mt-4 space-y-6">
        <div class="flex items-center justify-between">
          <div>
            <Label>Automatically Keep Up to Date</Label>
            <p class="text-xs text-muted-foreground">Download and install new DevPod versions in background</p>
          </div>
          <Switch checked={$autoUpdate} onCheckedChange={(v) => setAutoUpdate(v)} />
        </div>

        <Separator />

        <div class="space-y-2">
          <h2 class="text-lg font-semibold">Current Version</h2>
          {#if cliVersion}
            <p class="text-sm">DevPod CLI: <span class="font-mono">{cliVersion}</span></p>
          {:else}
            <p class="text-sm text-muted-foreground">Version information not available</p>
          {/if}
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
          <Switch checked={opts.experimentalMultiDevcontainer} onCheckedChange={() => toggleContextOption("experimentalMultiDevcontainer")} disabled={loading || saving} />
        </div>

        <Separator />

        <div class="space-y-2">
          <Label>Additional CLI Flags</Label>
          <p class="text-xs text-muted-foreground">Append custom flags to all DevPod CLI commands</p>
          <Input
            value={opts.additionalCliFlags}
            placeholder="--flag1 --flag2=value"
            oninput={(e) => (opts.additionalCliFlags = e.currentTarget.value)}
            onblur={() => saveContextOption("additionalCliFlags", opts.additionalCliFlags)}
            disabled={loading || saving}
          />
        </div>

        <div class="space-y-2">
          <Label>Additional Environment Variables</Label>
          <p class="text-xs text-muted-foreground">Comma-separated environment variables passed to DevPod commands</p>
          <Input
            value={opts.additionalEnvVars}
            placeholder="FOO=bar,BAZ=false"
            oninput={(e) => (opts.additionalEnvVars = e.currentTarget.value)}
            onblur={() => saveContextOption("additionalEnvVars", opts.additionalEnvVars)}
            disabled={loading || saving}
          />
        </div>
      </div>
    </Tabs.Content>
  </Tabs.Root>
</div>
