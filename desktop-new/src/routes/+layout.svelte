<script lang="ts">
import "../app.css"
import { onMount, onDestroy } from "svelte"
import Sidebar from "$lib/components/layout/Sidebar.svelte"
import ThemeSwitcher from "$lib/components/layout/ThemeSwitcher.svelte"
import { initWorkspaces, destroyWorkspaces } from "$lib/stores/workspaces.js"
import { initProviders, destroyProviders } from "$lib/stores/providers.js"
import { initMachines, destroyMachines } from "$lib/stores/machines.js"
import { initSettings } from "$lib/stores/settings.js"
import { terminalCount } from "$lib/stores/terminals.js"

let { children } = $props()

let destroySettings: (() => void) | undefined

onMount(() => {
  initWorkspaces()
  initProviders()
  initMachines()
  destroySettings = initSettings()
})

onDestroy(() => {
  destroyWorkspaces()
  destroyProviders()
  destroyMachines()
  destroySettings?.()
})
</script>

<div class="flex h-screen overflow-hidden">
  <Sidebar terminalCount={$terminalCount} />

  <div class="flex flex-1 flex-col overflow-hidden">
    <header class="flex h-12 items-center justify-end border-b px-4">
      <ThemeSwitcher />
    </header>

    <main class="flex-1 overflow-auto p-6">
      {@render children()}
    </main>
  </div>
</div>
