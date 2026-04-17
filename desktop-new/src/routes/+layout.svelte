<script lang="ts">
import "../app.css"
import { goto } from "$app/navigation"
import { onMount, onDestroy } from "svelte"
import Sidebar from "$lib/components/layout/Sidebar.svelte"
import ThemeSwitcher from "$lib/components/layout/ThemeSwitcher.svelte"
import NotificationHistory from "$lib/components/layout/NotificationHistory.svelte"
import { Toaster } from "$lib/components/ui/sonner/index.js"
import CommandPalette from "$lib/components/layout/CommandPalette.svelte"
import Breadcrumbs from "$lib/components/layout/Breadcrumbs.svelte"
import * as SidebarUI from "$lib/components/ui/sidebar/index.js"
import { initWorkspaces, destroyWorkspaces } from "$lib/stores/workspaces.js"
import { initProviders, destroyProviders } from "$lib/stores/providers.js"
import { initMachines, destroyMachines } from "$lib/stores/machines.js"
import { initContexts, destroyContexts } from "$lib/stores/contexts.js"
import { initSettings } from "$lib/stores/settings.js"
import { terminalCount } from "$lib/stores/terminals.js"
import { togglePalette } from "$lib/stores/command-palette.js"
import { appReady } from "$lib/ipc/commands.js"
import { isTauri } from "$lib/ipc/mock.js"

let { children } = $props()

let destroySettings: (() => void) | undefined

const NAV_KEYS: Record<string, string> = {
  "1": "/",
  "2": "/workspaces",
  "3": "/providers",
  "4": "/machines",
  "5": "/terminals",
  "6": "/ssh-keys",
  "7": "/settings",
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

  // Signal Tauri that the frontend is ready — closes splash, shows main window
  if (isTauri()) {
    import("@tauri-apps/api/core")
      .then(({ invoke }) => invoke("app_ready"))
      .catch((err) => {
        console.warn("[DevPod] appReady failed:", err)
      })
  }
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

<SidebarUI.Provider>
  <Sidebar terminalCount={$terminalCount} />

  <SidebarUI.Inset>
    <header class="flex h-12 items-center justify-between border-b px-4">
      <div class="flex items-center gap-2">
        <SidebarUI.Trigger class="-ml-1" />
        <Breadcrumbs />
      </div>
      <div class="ml-auto flex items-center gap-1">
        <NotificationHistory />
        <ThemeSwitcher />
      </div>
    </header>

    <main class="flex-1 overflow-auto p-6">
      {@render children()}
    </main>
  </SidebarUI.Inset>

  <Toaster richColors closeButton position="bottom-right" />
  <CommandPalette />
</SidebarUI.Provider>
