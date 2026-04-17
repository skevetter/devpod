<script lang="ts">
import { onMount } from "svelte"
import { Button } from "$lib/components/ui/button/index.js"
import { Input } from "$lib/components/ui/input/index.js"
import { Label } from "$lib/components/ui/label/index.js"
import { Separator } from "$lib/components/ui/separator/index.js"
import * as Select from "$lib/components/ui/select/index.js"
import { badgeVariants } from "$lib/components/ui/badge/index.js"
import * as Sheet from "$lib/components/ui/sheet/index.js"
import ConfirmDialog from "$lib/components/layout/ConfirmDialog.svelte"
import {
  providerUse,
  providerUpdate,
  providerDelete,
  providerOptions,
  providerSetOptions,
} from "$lib/ipc/commands.js"
import { toasts } from "$lib/stores/toasts.js"
import type { Provider, ProviderOption } from "$lib/types/index.js"

let {
  provider,
  open = $bindable(false),
  setup = false,
  ondeleted,
}: {
  provider: Provider
  open: boolean
  setup?: boolean
  ondeleted?: () => void
} = $props()

let options = $state<Record<string, ProviderOption>>({})
let optionValues = $state<Record<string, string>>({})
let initialValues = $state<Record<string, string>>({})
let saving = $state(false)
let loading = $state(true)
let confirmDeleteOpen = $state(false)
let deleting = $state(false)
let setupCompleted = $state(false)

let isDirty = $derived.by(() => {
  for (const key of Object.keys(optionValues)) {
    if (optionValues[key] !== (initialValues[key] ?? "")) return true
  }
  return false
})

let requiredOptions = $derived.by(() => {
  return Object.entries(options).filter(
    ([, opt]) => opt.required && !opt.hidden,
  )
})

let hasUnfilledRequired = $derived.by(() => {
  return requiredOptions.some(([key]) => !optionValues[key]?.trim())
})

let groupedOptions = $derived.by(() => {
  const groups: Record<string, [string, ProviderOption][]> = {}
  for (const [key, opt] of Object.entries(options)) {
    if (opt.hidden) continue
    const group = opt.group ?? ""
    if (!groups[group]) groups[group] = []
    groups[group].push([key, opt])
  }
  for (const entries of Object.values(groups)) {
    entries.sort((a, b) => {
      const aReq = a[1].required ? 0 : 1
      const bReq = b[1].required ? 0 : 1
      return aReq - bReq
    })
  }
  return groups
})

async function loadOptions() {
  loading = true
  try {
    const raw = await providerOptions(provider.name)
    options = raw as unknown as Record<string, ProviderOption>

    const currentOpts = provider.options ?? {}
    for (const [key, opt] of Object.entries(options)) {
      const currentVal = currentOpts[key]
      if (currentVal?.value != null) {
        optionValues[key] = String(currentVal.value)
      } else if (opt.default != null) {
        optionValues[key] = String(opt.default)
      } else {
        optionValues[key] = ""
      }
    }
    initialValues = { ...optionValues }
  } catch (err) {
    toasts.error(`Failed to load options: ${err}`)
  } finally {
    loading = false
  }
}

$effect(() => {
  if (open) {
    loadOptions()
  }
})

async function handleSetDefault() {
  try {
    await providerUse(provider.name)
    toasts.success(`Set ${provider.name} as default provider`)
  } catch (err) {
    toasts.error(`Failed to set default: ${err}`)
  }
}

async function handleUpdate() {
  try {
    await providerUpdate(provider.name)
    toasts.success(`Updated ${provider.name}`)
  } catch (err) {
    toasts.error(`Failed to update: ${err}`)
  }
}

async function handleDelete() {
  deleting = true
  try {
    await providerDelete(provider.name)
    toasts.success(`Deleted ${provider.name}`)
    confirmDeleteOpen = false
    open = false
    ondeleted?.()
  } catch (err) {
    toasts.error(`Failed to delete: ${err}`)
  } finally {
    deleting = false
  }
}

async function handleSaveOptions() {
  const missing = requiredOptions
    .filter(([key]) => !optionValues[key]?.trim())
    .map(([, opt]) => opt.displayName ?? opt.name ?? "")
  if (missing.length > 0) {
    toasts.error(`Required: ${missing.join(", ")}`)
    return
  }
  saving = true
  try {
    const values: Record<string, string> = {}
    for (const [key, val] of Object.entries(optionValues)) {
      if (val !== "") values[key] = val
    }
    await providerSetOptions(provider.name, values)
    initialValues = { ...optionValues }
    if (setup) {
      await providerUse(provider.name)
      setupCompleted = true
      toasts.success(`Provider ${provider.name} configured successfully`)
      open = false
    } else {
      toasts.success("Options saved")
    }
  } catch (err) {
    toasts.error(`Failed to save options: ${err}`)
  } finally {
    saving = false
  }
}
</script>

<Sheet.Root bind:open onOpenChange={async (isOpen) => {
  if (!isOpen && setup && !setupCompleted) {
    try {
      await providerDelete(provider.name)
      ondeleted?.()
    } catch {
      // Provider may already be gone
    }
  }
}}>
  <Sheet.ResizableContent>
    <Sheet.Header class="p-6">
      <Sheet.Title class="flex items-center gap-2">
        {provider.name}
        {#if provider.version}
          <span class={badgeVariants({ variant: "outline" })}>{provider.version}</span>
        {/if}
        {#if provider.isDefault}
          <span class={badgeVariants({ variant: "default" })}>default</span>
        {/if}
        {#if provider.state?.initialized}
          <span class={badgeVariants({ variant: "secondary" })}>initialized</span>
        {/if}
      </Sheet.Title>
      {#if provider.description}
        <Sheet.Description>{provider.description}</Sheet.Description>
      {/if}
    </Sheet.Header>

    <div class="flex gap-2 px-6">
      <Button variant="outline" size="sm" onclick={handleSetDefault}>Set Default</Button>
      <Button variant="outline" size="sm" onclick={handleUpdate}>Update</Button>
      <Button variant="destructive" size="sm" onclick={() => (confirmDeleteOpen = true)}>Delete</Button>
    </div>

    <Separator />

    <div class="flex-1 overflow-y-auto space-y-4 px-6 pb-6">
      {#if setup && !loading && hasUnfilledRequired}
        <div class="rounded-md border border-amber-500/50 bg-amber-500/10 p-3">
          <h3 class="text-sm font-semibold text-amber-700 dark:text-amber-400">Configure required options</h3>
          <p class="mt-1 text-xs text-amber-600 dark:text-amber-400/80">
            Fill in the required fields below and save.
          </p>
        </div>
      {/if}

      {#if loading}
        <p class="text-sm text-muted-foreground">Loading options...</p>
      {:else if Object.keys(options).length === 0}
        <p class="text-sm text-muted-foreground">No configurable options available.</p>
      {:else}
        {#each Object.entries(groupedOptions) as [group, entries] (group)}
          {#if group}
            <h3 class="text-xs font-medium text-muted-foreground uppercase tracking-wider pt-2">
              {group}
            </h3>
          {/if}

          {#each entries as [key, opt] (key)}
            <div class="space-y-1.5 {setup && opt.required && !optionValues[key]?.trim() ? 'rounded-md border border-amber-500/30 bg-amber-500/5 p-3' : ''}">
              <Label class="text-sm">
                {opt.displayName ?? opt.name ?? key}
                {#if opt.required}
                  <span class="text-destructive">*</span>
                {/if}
              </Label>
              {#if opt.description}
                <p class="text-xs text-muted-foreground">{opt.description}</p>
              {/if}
              {#if opt.enum && opt.enum.length > 0}
                <Select.Root
                  type="single"
                  value={optionValues[key] ?? ""}
                  onValueChange={(v) => (optionValues[key] = v)}
                >
                  <Select.Trigger class="w-full h-9">
                    <span>{optionValues[key] || "-- Select --"}</span>
                  </Select.Trigger>
                  <Select.Content>
                    {#each opt.enum as enumVal}
                      <Select.Item value={enumVal} label={enumVal} />
                    {/each}
                  </Select.Content>
                </Select.Root>
              {:else}
                <Input
                  type={opt.password ? "password" : "text"}
                  placeholder={opt.default != null ? String(opt.default) : ""}
                  value={optionValues[key] ?? ""}
                  oninput={(e) => (optionValues[key] = e.currentTarget.value)}
                  class="h-9"
                />
              {/if}
            </div>
          {/each}
        {/each}
      {/if}
    </div>

    <Sheet.Footer class="p-6">
      <div class="flex items-center gap-2">
        <Button onclick={handleSaveOptions} disabled={saving || !isDirty} size="sm">
          {saving ? "Saving..." : setup ? "Save & Finish" : "Save Options"}
        </Button>
        {#if isDirty}
          <Button variant="outline" size="sm" onclick={() => { optionValues = { ...initialValues } }}>
            Reset
          </Button>
          <span class="text-xs text-muted-foreground">Unsaved changes</span>
        {/if}
      </div>
    </Sheet.Footer>
  </Sheet.ResizableContent>
</Sheet.Root>

<ConfirmDialog
  bind:open={confirmDeleteOpen}
  title="Delete provider"
  description="This will remove provider '{provider.name}' and its configuration. Any workspaces using this provider will need a new one."
  confirmLabel="Delete"
  loading={deleting}
  onconfirm={handleDelete}
/>
