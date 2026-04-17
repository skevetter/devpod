<script lang="ts">
import { page } from "$app/stores"
import { goto } from "$app/navigation"
import { onMount } from "svelte"
import { Button } from "$lib/components/ui/button/index.js"
import { Input } from "$lib/components/ui/input/index.js"
import { Label } from "$lib/components/ui/label/index.js"
import { Separator } from "$lib/components/ui/separator/index.js"
import * as Select from "$lib/components/ui/select/index.js"
import { badgeVariants } from "$lib/components/ui/badge/index.js"
import ConfirmDialog from "$lib/components/layout/ConfirmDialog.svelte"
import { providers } from "$lib/stores/providers.js"
import {
  providerUse,
  providerUpdate,
  providerDelete,
  providerOptions,
  providerSetOptions,
} from "$lib/ipc/commands.js"
import { toasts } from "$lib/stores/toasts.js"
import type { ProviderOption } from "$lib/types/index.js"

let id = $derived($page.params.id as string)
let provider = $derived($providers.find((p) => p.name === id))
let isSetup = $derived($page.url.searchParams.get("setup") === "true")
let isInitialized = $derived(provider?.state?.initialized === true)

let options = $state<Record<string, ProviderOption>>({})
let optionValues = $state<Record<string, string>>({})
let initialValues = $state<Record<string, string>>({})
let saving = $state(false)
let loading = $state(true)
let confirmDeleteOpen = $state(false)
let deleting = $state(false)

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

// Group options by their group field, with ungrouped options first
// In setup mode, show required options first regardless of group
let groupedOptions = $derived.by(() => {
  const groups: Record<string, [string, ProviderOption][]> = {}
  for (const [key, opt] of Object.entries(options)) {
    if (opt.hidden) continue
    const group = opt.group ?? ""
    if (!groups[group]) groups[group] = []
    groups[group].push([key, opt])
  }
  // Sort required options first within each group
  for (const entries of Object.values(groups)) {
    entries.sort((a, b) => {
      const aReq = a[1].required ? 0 : 1
      const bReq = b[1].required ? 0 : 1
      return aReq - bReq
    })
  }
  return groups
})

onMount(async () => {
  try {
    const raw = await providerOptions(id)
    options = raw as unknown as Record<string, ProviderOption>

    // Initialize values: prefer current provider option values, then defaults
    const currentOpts = provider?.options ?? {}
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
  } catch {
    // Options not available
  } finally {
    loading = false
  }
})

async function handleSetDefault() {
  try {
    await providerUse(id)
    toasts.success(`Set ${id} as default provider`)
  } catch (err) {
    toasts.error(`Failed to set default: ${err}`)
  }
}

async function handleUpdate() {
  try {
    await providerUpdate(id)
    toasts.success(`Updated ${id}`)
  } catch (err) {
    toasts.error(`Failed to update: ${err}`)
  }
}

async function handleDelete() {
  deleting = true
  try {
    await providerDelete(id)
    toasts.success(`Deleted ${id}`)
    confirmDeleteOpen = false
    goto("/providers")
  } catch (err) {
    toasts.error(`Failed to delete: ${err}`)
  } finally {
    deleting = false
  }
}

async function handleSaveOptions() {
  saving = true
  try {
    const values: Record<string, string> = {}
    for (const [key, val] of Object.entries(optionValues)) {
      if (val !== "") values[key] = val
    }
    await providerSetOptions(id, values)
    initialValues = { ...optionValues }
    if (isSetup) {
      toasts.success(`Provider ${id} configured successfully`)
      goto("/providers")
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

<div class="space-y-6">
  <div class="flex items-center gap-4">
    <Button variant="ghost" size="sm" onclick={() => goto("/providers")}>
      &larr; Back
    </Button>
    <h1 class="text-2xl font-bold">{id}</h1>
    {#if provider?.version}
      <span class={badgeVariants({ variant: "secondary" })}>{provider.version}</span>
    {/if}
    {#if provider?.state?.initialized}
      <span class={badgeVariants({ variant: "default" })}>initialized</span>
    {/if}
  </div>

  <div class="flex gap-2">
    <Button variant="outline" size="sm" onclick={handleSetDefault}>Set Default</Button>
    <Button variant="outline" size="sm" onclick={handleUpdate}>Update</Button>
    <Button variant="destructive" size="sm" onclick={() => (confirmDeleteOpen = true)}>Delete</Button>
  </div>

  {#if !provider}
    <p class="text-muted-foreground">Provider not found.</p>
  {:else}
    {#if isSetup && !loading && hasUnfilledRequired}
      <div class="rounded-md border border-amber-500/50 bg-amber-500/10 p-4">
        <h3 class="font-semibold text-amber-700 dark:text-amber-400">Configure required options</h3>
        <p class="mt-1 text-sm text-amber-600 dark:text-amber-400/80">
          This provider needs configuration before it can be used.
          Fill in the required fields below and save.
        </p>
      </div>
    {:else if !isInitialized && !loading}
      <div class="rounded-md border border-muted bg-muted/50 p-3 text-sm text-muted-foreground">
        This provider has not been initialized yet. Configure its options to get started.
      </div>
    {/if}

    {#if provider.description}
      <p class="text-muted-foreground">{provider.description}</p>
    {/if}

    <Separator />

    <div class="space-y-4">
      <h2 class="text-lg font-semibold">
        {isSetup ? "Configure Provider" : "Options"}
      </h2>

      {#if loading}
        <p class="text-sm text-muted-foreground">Loading options...</p>
      {:else if Object.keys(options).length === 0}
        <p class="text-sm text-muted-foreground">No configurable options available.</p>
      {:else}
        {#each Object.entries(groupedOptions) as [group, entries] (group)}
          {#if group}
            <h3 class="mt-4 text-sm font-medium text-muted-foreground uppercase tracking-wider">
              {group}
            </h3>
          {/if}

          <div class="space-y-4">
            {#each entries as [key, opt] (key)}
              <div class="space-y-1.5 {isSetup && opt.required && !optionValues[key]?.trim() ? 'rounded-md border border-amber-500/30 bg-amber-500/5 p-3 -mx-3' : ''}"  >
                <Label>
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
                    <Select.Trigger class="w-full">
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
                  />
                {/if}
              </div>
            {/each}
          </div>
        {/each}

        <div class="flex items-center gap-3">
          <Button onclick={handleSaveOptions} disabled={saving || !isDirty}>
            {saving ? "Saving..." : isSetup ? "Save & Finish Setup" : "Save Options"}
          </Button>
          {#if isSetup && !hasUnfilledRequired}
            <Button variant="outline" onclick={() => goto("/providers")}>
              Skip
            </Button>
          {:else if isDirty}
            <Button variant="outline" onclick={() => { optionValues = { ...initialValues } }}>
              Reset
            </Button>
            <span class="text-xs text-muted-foreground">Unsaved changes</span>
          {/if}
        </div>
      {/if}
    </div>
  {/if}
</div>

<ConfirmDialog
  bind:open={confirmDeleteOpen}
  title="Delete provider"
  description="This will remove provider '{id}' and its configuration. Any workspaces using this provider will need a new one."
  confirmLabel="Delete"
  loading={deleting}
  onconfirm={handleDelete}
/>
