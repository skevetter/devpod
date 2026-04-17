<script lang="ts">
import { Info } from "@lucide/svelte"
import { Button } from "$lib/components/ui/button/index.js"
import { Input } from "$lib/components/ui/input/index.js"
import { Label } from "$lib/components/ui/label/index.js"
import { Separator } from "$lib/components/ui/separator/index.js"
import { Switch } from "$lib/components/ui/switch/index.js"
import { badgeVariants } from "$lib/components/ui/badge/index.js"
import * as Alert from "$lib/components/ui/alert/index.js"
import * as Sheet from "$lib/components/ui/sheet/index.js"
import {
  contextOptions as fetchContextOptions,
  contextSetOptions,
} from "$lib/ipc/commands.js"
import {
  parseContextOptions,
  CONTEXT_OPTION_KEYS,
} from "$lib/stores/settings.js"
import type { ContextOptions } from "$lib/stores/settings.js"
import { toasts } from "$lib/stores/toasts.js"
import type { Context } from "$lib/types/index.js"

let {
  context,
  isActive = false,
  open = $bindable(false),
}: {
  context: Context
  isActive: boolean
  open: boolean
} = $props()

let opts = $state<ContextOptions>({
  telemetry: true,
  agentUrl: "",
  dotfilesUrl: "",
  dotfilesScript: "",
  dockerCredentialForwarding: true,
  gitCredentialForwarding: true,
  gitSshSignatureForwarding: true,
  sshAgentForwarding: true,
  sshAddPrivateKeys: true,
  sshStrictHostKeyChecking: false,
  gpgAgentForwarding: false,
  agentInjectTimeout: "20",
  registryCache: "",
  exitAfterTimeout: true,
  sshConfigPath: "",
  sshConfigIncludePath: "",
})

let loading = $state(true)
let saving = $state(false)

async function loadOptions() {
  loading = true
  try {
    const raw = await fetchContextOptions(context.name)
    opts = parseContextOptions(raw)
  } catch {
    // Keep defaults
  } finally {
    loading = false
  }
}

$effect(() => {
  if (open) {
    loadOptions()
  }
})

async function saveOption(key: keyof ContextOptions, value: string | boolean) {
  saving = true
  const cliKey = CONTEXT_OPTION_KEYS[key]
  try {
    await contextSetOptions([`${cliKey}=${String(value)}`], context.name)
    ;(opts as unknown as Record<string, string | boolean>)[key] = value
    toasts.success("Option saved")
  } catch (err) {
    toasts.error(`Failed to save: ${err}`)
  } finally {
    saving = false
  }
}

function toggleOption(key: keyof ContextOptions) {
  saveOption(key, !opts[key])
}
</script>

<Sheet.Root bind:open>
  <Sheet.ResizableContent>
    <Sheet.Header class="p-6">
      <Sheet.Title class="flex items-center gap-2">
        {context.name}
        {#if isActive}
          <span class={badgeVariants({ variant: "default" })}>active</span>
        {/if}
      </Sheet.Title>
      <Sheet.Description>Configure context options for {context.name}</Sheet.Description>
    </Sheet.Header>

    <div class="flex-1 overflow-y-auto space-y-5 px-6 pb-6">
      {#if loading}
        <p class="text-sm text-muted-foreground">Loading options...</p>
      {:else}
        {#if !isActive}
          <Alert.Root>
            <Info class="size-4" />
            <Alert.AlertTitle>Inactive context</Alert.AlertTitle>
            <Alert.AlertDescription>
              Changes are saved to this context but only take effect when it is the active context.
            </Alert.AlertDescription>
          </Alert.Root>
        {/if}

        <!-- General -->
        <div class="space-y-4">
          <h3 class="text-sm font-semibold text-muted-foreground uppercase tracking-wider">General</h3>

          <div class="flex items-center justify-between">
            <div>
              <Label>Telemetry</Label>
              <p class="text-xs text-muted-foreground">Send anonymous usage data</p>
            </div>
            <Switch checked={opts.telemetry} onCheckedChange={() => toggleOption("telemetry")} disabled={saving} />
          </div>

          <div class="space-y-1.5">
            <Label>Agent URL</Label>
            <p class="text-xs text-muted-foreground">Custom agent endpoint URL</p>
            <Input
              value={opts.agentUrl}
              placeholder="Leave empty for default"
              oninput={(e) => (opts.agentUrl = e.currentTarget.value)}
              onblur={() => saveOption("agentUrl", opts.agentUrl)}
              disabled={saving}
              class="h-9"
            />
          </div>

          <div class="space-y-1.5">
            <Label>Agent Inject Timeout</Label>
            <p class="text-xs text-muted-foreground">Seconds to wait for agent injection</p>
            <Input
              value={opts.agentInjectTimeout}
              placeholder="20"
              oninput={(e) => (opts.agentInjectTimeout = e.currentTarget.value)}
              onblur={() => saveOption("agentInjectTimeout", opts.agentInjectTimeout)}
              disabled={saving}
              class="h-9 max-w-24"
            />
          </div>

          <div class="space-y-1.5">
            <Label>Registry Cache</Label>
            <p class="text-xs text-muted-foreground">Registry mirror or cache URL</p>
            <Input
              value={opts.registryCache}
              placeholder="Leave empty for default"
              oninput={(e) => (opts.registryCache = e.currentTarget.value)}
              onblur={() => saveOption("registryCache", opts.registryCache)}
              disabled={saving}
              class="h-9"
            />
          </div>

          <div class="flex items-center justify-between">
            <div>
              <Label>Exit After Timeout</Label>
              <p class="text-xs text-muted-foreground">Shut down workspace after inactivity timeout</p>
            </div>
            <Switch checked={opts.exitAfterTimeout} onCheckedChange={() => toggleOption("exitAfterTimeout")} disabled={saving} />
          </div>
        </div>

        <Separator />

        <!-- Dotfiles -->
        <div class="space-y-4">
          <h3 class="text-sm font-semibold text-muted-foreground uppercase tracking-wider">Dotfiles</h3>

          <div class="space-y-1.5">
            <Label>Repository</Label>
            <p class="text-xs text-muted-foreground">Git repository URL for dotfiles to apply in workspaces</p>
            <Input
              value={opts.dotfilesUrl}
              placeholder="https://github.com/user/dotfiles"
              oninput={(e) => (opts.dotfilesUrl = e.currentTarget.value)}
              onblur={() => saveOption("dotfilesUrl", opts.dotfilesUrl)}
              disabled={saving}
              class="h-9"
            />
          </div>

          <div class="space-y-1.5">
            <Label>Install Script</Label>
            <p class="text-xs text-muted-foreground">Script to run after cloning dotfiles repository</p>
            <Input
              value={opts.dotfilesScript}
              placeholder="install.sh"
              oninput={(e) => (opts.dotfilesScript = e.currentTarget.value)}
              onblur={() => saveOption("dotfilesScript", opts.dotfilesScript)}
              disabled={saving}
              class="h-9"
            />
          </div>
        </div>

        <Separator />

        <!-- SSH -->
        <div class="space-y-4">
          <h3 class="text-sm font-semibold text-muted-foreground uppercase tracking-wider">SSH</h3>

          <div class="flex items-center justify-between">
            <div>
              <Label>SSH Agent Forwarding</Label>
              <p class="text-xs text-muted-foreground">Forward SSH agent to workspaces</p>
            </div>
            <Switch checked={opts.sshAgentForwarding} onCheckedChange={() => toggleOption("sshAgentForwarding")} disabled={saving} />
          </div>

          <div class="flex items-center justify-between">
            <div>
              <Label>SSH Add Private Keys</Label>
              <p class="text-xs text-muted-foreground">Automatically add private SSH keys to agent</p>
            </div>
            <Switch checked={opts.sshAddPrivateKeys} onCheckedChange={() => toggleOption("sshAddPrivateKeys")} disabled={saving} />
          </div>

          <div class="flex items-center justify-between">
            <div>
              <Label>Strict Host Key Checking</Label>
              <p class="text-xs text-muted-foreground">Enable strict SSH host key verification</p>
            </div>
            <Switch checked={opts.sshStrictHostKeyChecking} onCheckedChange={() => toggleOption("sshStrictHostKeyChecking")} disabled={saving} />
          </div>

          <div class="space-y-1.5">
            <Label>SSH Config Path</Label>
            <p class="text-xs text-muted-foreground">Path to SSH config file</p>
            <Input
              value={opts.sshConfigPath}
              placeholder="~/.ssh/config"
              oninput={(e) => (opts.sshConfigPath = e.currentTarget.value)}
              onblur={() => saveOption("sshConfigPath", opts.sshConfigPath)}
              disabled={saving}
              class="h-9"
            />
          </div>

          <div class="space-y-1.5">
            <Label>SSH Config Include Path</Label>
            <p class="text-xs text-muted-foreground">Path for SSH config includes</p>
            <Input
              value={opts.sshConfigIncludePath}
              placeholder="~/.ssh/devpod_config"
              oninput={(e) => (opts.sshConfigIncludePath = e.currentTarget.value)}
              onblur={() => saveOption("sshConfigIncludePath", opts.sshConfigIncludePath)}
              disabled={saving}
              class="h-9"
            />
          </div>
        </div>

        <Separator />

        <!-- Credential Forwarding -->
        <div class="space-y-4">
          <h3 class="text-sm font-semibold text-muted-foreground uppercase tracking-wider">Credential Forwarding</h3>

          <div class="flex items-center justify-between">
            <div>
              <Label>Docker Credentials</Label>
              <p class="text-xs text-muted-foreground">Forward Docker credentials to workspaces</p>
            </div>
            <Switch checked={opts.dockerCredentialForwarding} onCheckedChange={() => toggleOption("dockerCredentialForwarding")} disabled={saving} />
          </div>

          <div class="flex items-center justify-between">
            <div>
              <Label>Git Credentials</Label>
              <p class="text-xs text-muted-foreground">Forward Git credential helper to workspaces</p>
            </div>
            <Switch checked={opts.gitCredentialForwarding} onCheckedChange={() => toggleOption("gitCredentialForwarding")} disabled={saving} />
          </div>

          <div class="flex items-center justify-between">
            <div>
              <Label>Git SSH Signature</Label>
              <p class="text-xs text-muted-foreground">Forward Git SSH signature to workspaces</p>
            </div>
            <Switch checked={opts.gitSshSignatureForwarding} onCheckedChange={() => toggleOption("gitSshSignatureForwarding")} disabled={saving} />
          </div>

          <div class="flex items-center justify-between">
            <div>
              <Label>GPG Agent</Label>
              <p class="text-xs text-muted-foreground">Forward GPG agent to workspaces</p>
            </div>
            <Switch checked={opts.gpgAgentForwarding} onCheckedChange={() => toggleOption("gpgAgentForwarding")} disabled={saving} />
          </div>
        </div>
      {/if}
    </div>
  </Sheet.ResizableContent>
</Sheet.Root>
