<script lang="ts">
import { KeyRound, Copy, Plus, ShieldCheck, ShieldAlert } from "@lucide/svelte"
import { onMount } from "svelte"
import { Button } from "$lib/components/ui/button/index.js"
import { Input } from "$lib/components/ui/input/index.js"
import { Label } from "$lib/components/ui/label/index.js"
import { Separator } from "$lib/components/ui/separator/index.js"
import * as Select from "$lib/components/ui/select/index.js"
import { badgeVariants } from "$lib/components/ui/badge/index.js"
import { ScrollArea } from "$lib/components/ui/scroll-area/index.js"
import CardSkeleton from "$lib/components/ui/skeleton/CardSkeleton.svelte"
import { sshKeyList, sshKeyGenerate } from "$lib/ipc/commands.js"
import { toasts } from "$lib/stores/toasts.js"
import type { SshKeyInfo } from "$lib/types/index.js"

let keys = $state<SshKeyInfo[]>([])
let loading = $state(true)

let showGenerate = $state(false)
let newKeyName = $state("")
let newKeyType = $state("ed25519")
let newKeyComment = $state("")
let generating = $state(false)
let generateError = $state("")

let copiedKey = $state<string | null>(null)

onMount(async () => {
  await loadKeys()
})

async function loadKeys() {
  loading = true
  try {
    keys = await sshKeyList()
  } catch {
    keys = []
  } finally {
    loading = false
  }
}

async function handleGenerate() {
  if (!newKeyName.trim()) {
    generateError = "Key name is required"
    return
  }
  if (!/^[a-zA-Z0-9_-]+$/.test(newKeyName)) {
    generateError =
      "Name can only contain letters, numbers, hyphens, and underscores"
    return
  }

  generateError = ""
  generating = true
  try {
    const key = await sshKeyGenerate({
      name: newKeyName.trim(),
      keyType: newKeyType,
      comment: newKeyComment.trim() || undefined,
    })
    keys = [...keys, key].sort((a, b) => a.name.localeCompare(b.name))
    toasts.success(`Generated SSH key: ${key.name}`)
    showGenerate = false
    newKeyName = ""
    newKeyComment = ""
  } catch (err) {
    generateError = err instanceof Error ? err.message : String(err)
  } finally {
    generating = false
  }
}

async function copyPublicKey(key: SshKeyInfo) {
  try {
    await navigator.clipboard.writeText(key.publicKey)
    copiedKey = key.name
    toasts.success(`Copied ${key.name} public key`)
    setTimeout(() => {
      if (copiedKey === key.name) copiedKey = null
    }, 2000)
  } catch {
    toasts.error("Failed to copy to clipboard")
  }
}
</script>

<div class="space-y-6">
  <div class="flex items-center justify-between">
    <h1 class="text-2xl font-bold">SSH Keys</h1>
    <Button onclick={() => { showGenerate = !showGenerate }}>
      <Plus class="mr-1 h-4 w-4" />
      Generate Key
    </Button>
  </div>

  {#if showGenerate}
    <div class="rounded-lg border bg-card p-4 shadow-sm space-y-4">
      <h2 class="text-lg font-semibold">Generate New SSH Key</h2>

      {#if generateError}
        <div class="rounded-md border border-destructive bg-destructive/10 p-3 text-sm text-destructive">
          {generateError}
        </div>
      {/if}

      <div class="grid gap-4 sm:grid-cols-2">
        <div class="space-y-2">
          <Label>Key Name</Label>
          <Input
            placeholder="e.g. devpod-aws"
            value={newKeyName}
            oninput={(e) => (newKeyName = e.currentTarget.value)}
          />
        </div>
        <div class="space-y-2">
          <Label>Key Type</Label>
          <Select.Root type="single" bind:value={newKeyType}>
            <Select.Trigger class="w-full">
              <span>
                {newKeyType === "ed25519" ? "Ed25519 (recommended)" : newKeyType === "rsa" ? "RSA (4096-bit)" : "ECDSA"}
              </span>
            </Select.Trigger>
            <Select.Content>
              <Select.Item value="ed25519" label="Ed25519 (recommended)" />
              <Select.Item value="rsa" label="RSA (4096-bit)" />
              <Select.Item value="ecdsa" label="ECDSA" />
            </Select.Content>
          </Select.Root>
        </div>
      </div>

      <div class="space-y-2">
        <Label>Comment (optional)</Label>
        <Input
          placeholder="e.g. your-email@example.com"
          value={newKeyComment}
          oninput={(e) => (newKeyComment = e.currentTarget.value)}
        />
      </div>

      <div class="flex gap-2">
        <Button onclick={handleGenerate} disabled={generating}>
          {generating ? "Generating..." : "Generate"}
        </Button>
        <Button variant="outline" onclick={() => { showGenerate = false; generateError = "" }}>
          Cancel
        </Button>
      </div>
    </div>
  {/if}

  <Separator />

  {#if loading}
    <div class="space-y-3">
      {#each Array(2) as _}
        <CardSkeleton />
      {/each}
    </div>
  {:else if keys.length === 0}
    <div class="flex flex-col items-center justify-center gap-4 py-16 text-center">
      <KeyRound class="h-10 w-10 text-muted-foreground" />
      <p class="text-muted-foreground">No SSH keys found in ~/.ssh</p>
      <Button onclick={() => { showGenerate = true }}>Generate your first key</Button>
    </div>
  {:else}
    <ScrollArea class="rounded-md border">
      <div class="divide-y">
        {#each keys as key (key.name)}
          <div class="flex items-start gap-4 p-4">
            <KeyRound class="mt-1 h-5 w-5 shrink-0 text-muted-foreground" />
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2">
                <span class="font-semibold">{key.name}</span>
                <span class={badgeVariants({ variant: "secondary" })}>{key.keyType}</span>
                {#if key.hasPassphrase}
                  <ShieldCheck class="h-4 w-4 text-green-600 dark:text-green-400" />
                {:else}
                  <ShieldAlert class="h-4 w-4 text-amber-500" />
                {/if}
              </div>
              {#if key.comment}
                <p class="mt-0.5 text-sm text-muted-foreground">{key.comment}</p>
              {/if}
              {#if key.fingerprint}
                <p class="mt-1 text-xs font-mono text-muted-foreground truncate">
                  {key.fingerprint}
                </p>
              {/if}
              <p class="mt-1 text-xs text-muted-foreground truncate">{key.path}</p>
            </div>
            <Button
              variant="outline"
              size="sm"
              onclick={() => copyPublicKey(key)}
            >
              <Copy class="mr-1 h-3 w-3" />
              {copiedKey === key.name ? "Copied!" : "Copy Public Key"}
            </Button>
          </div>
        {/each}
      </div>
    </ScrollArea>

    <p class="text-xs text-muted-foreground">
      {keys.length} key{keys.length === 1 ? "" : "s"} found.
      Keys with <ShieldCheck class="inline h-3 w-3 text-green-600" /> have a passphrase.
      Keys with <ShieldAlert class="inline h-3 w-3 text-amber-500" /> do not.
    </p>
  {/if}
</div>
