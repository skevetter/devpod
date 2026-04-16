<script lang="ts">
import "../app.css"
import { goto } from "$app/navigation"
import { onMount, onDestroy } from "svelte"
import Sidebar from "$lib/components/layout/Sidebar.svelte"
import ThemeSwitcher from "$lib/components/layout/ThemeSwitcher.svelte"
import Toaster from "$lib/components/layout/Toaster.svelte"
import CommandPalette from "$lib/components/layout/CommandPalette.svelte"
import Breadcrumbs from "$lib/components/layout/Breadcrumbs.svelte"
import { initWorkspaces, destroyWorkspaces } from "$lib/stores/workspaces.js"
import { initProviders, destroyProviders } from "$lib/stores/providers.js"
import { initMachines, destroyMachines } from "$lib/stores/machines.js"
import { initContexts, destroyContexts } from "$lib/stores/contexts.js"
import { initSettings } from "$lib/stores/settings.js"
import { terminalCount } from "$lib/stores/terminals.js"
import { togglePalette } from "$lib/stores/command-palette.js"

let { children } = $props()

let destroySettings: (() => void) | undefined

const NAV_KEYS: Record<string, string> = {
  "1": "/",
  "2": "/workspaces",
  "3": "/providers",
  "4": "/machines",
  "5": "/settings",
}

function handleKeydown(e: KeyboardEvent) {
  if ((e.metaKey || e.ctrlKey) && e.key === "k") {
    e.preventDefault()
    togglePalette()
    return
  }
  if ((e.metaKey || e.ctrlKey) && e.key === "n") {
    e.preventDefault()
    goto("/workspaces/new")
    return
  }
  if ((e.metaKey || e.ctrlKey) && NAV_KEYS[e.key]) {
    e.preventDefault()
    goto(NAV_KEYS[e.key])
  }
}

onMount(() => {
  initWorkspaces()
  initProviders()
  initMachines()
  initContexts()
  destroySettings = initSettings()
})

onDestroy(() => {
  destroyWorkspaces()
  destroyProviders()
  destroyMachines()
  destroyContexts()
  destroySettings?.()
})
</script>

<svelte:window onkeydown={handleKeydown} />

<div class="flex h-screen overflow-hidden">
  <Sidebar terminalCount={$terminalCount} />

  <div class="flex flex-1 flex-col overflow-hidden">
    <header class="flex h-12 items-center justify-between border-b px-4">
      <Breadcrumbs />
      <ThemeSwitcher />
    </header>

    <main class="flex-1 overflow-auto p-6">
      {@render children()}
    </main>
  </div>

  <Toaster />
  <CommandPalette />
</div>
