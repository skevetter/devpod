<script lang="ts">
import SidebarItem from "./SidebarItem.svelte"
import { Separator } from "$lib/components/ui/separator/index.js"
import { workspaces } from "$lib/stores/workspaces.js"
import { providers } from "$lib/stores/providers.js"
import { machines } from "$lib/stores/machines.js"
import { contexts } from "$lib/stores/contexts.js"
import { togglePalette } from "$lib/stores/command-palette.js"
import {
  LayoutDashboard,
  Box,
  Plug,
  Server,
  Layers,
  TerminalSquare,
  KeyRound,
  Settings,
  Search,
} from "lucide-svelte"

let { terminalCount = 0 }: { terminalCount?: number } = $props()
</script>

<aside class="flex h-full w-56 flex-col border-r bg-card">
  <div class="flex items-center px-4 py-4">
    <h1 class="text-lg font-bold tracking-tight text-foreground">DevPod</h1>
  </div>

  <Separator />

  <nav class="flex flex-1 flex-col gap-1 p-3">
    <SidebarItem href="/" label="Dashboard" icon={LayoutDashboard} />
    <SidebarItem href="/workspaces" label="Workspaces" badgeCount={$workspaces.length} icon={Box} />
    <SidebarItem href="/providers" label="Providers" badgeCount={$providers.length} icon={Plug} />
    <SidebarItem href="/machines" label="Machines" badgeCount={$machines.length} icon={Server} />
    <SidebarItem href="/contexts" label="Contexts" badgeCount={$contexts.length} icon={Layers} />
    <SidebarItem href="/terminals" label="Terminals" badgeCount={terminalCount} icon={TerminalSquare} />
    <SidebarItem href="/ssh-keys" label="SSH Keys" icon={KeyRound} />

    <div class="flex-1"></div>

    <Separator class="my-2" />
    <SidebarItem href="/settings" label="Settings" icon={Settings} />

    <button
      class="mt-1 flex items-center justify-between rounded-md px-3 py-2 text-sm text-muted-foreground transition-colors hover:bg-accent/50 hover:text-accent-foreground"
      onclick={togglePalette}
    >
      <span class="flex items-center gap-2">
        <Search class="h-4 w-4" />
        Search
      </span>
      <kbd class="rounded border bg-muted px-1.5 py-0.5 text-xs font-mono">&#8984;K</kbd>
    </button>
  </nav>
</aside>
