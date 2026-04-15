<script lang="ts">
import SidebarItem from "./SidebarItem.svelte"
import { Separator } from "$lib/components/ui/separator/index.js"
import { workspaces } from "$lib/stores/workspaces.js"
import { providers } from "$lib/stores/providers.js"
import { machines } from "$lib/stores/machines.js"
</script>

<aside class="flex h-full w-56 flex-col border-r bg-card">
  <div class="flex items-center px-4 py-4">
    <h1 class="text-lg font-bold tracking-tight text-foreground">DevPod</h1>
  </div>

  <Separator />

  <nav class="flex flex-1 flex-col gap-1 p-3">
    <SidebarItem href="/workspaces" label="Workspaces" badgeCount={$workspaces.length} />
    <SidebarItem href="/providers" label="Providers" badgeCount={$providers.length} />
    <SidebarItem href="/machines" label="Machines" badgeCount={$machines.length} />
    <SidebarItem href="/terminals" label="Terminals" />

    <div class="flex-1"></div>

    <Separator class="my-2" />
    <SidebarItem href="/settings" label="Settings" />
  </nav>
</aside>
