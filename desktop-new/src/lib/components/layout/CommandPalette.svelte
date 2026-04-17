<script lang="ts">
import { goto } from "$app/navigation"
import { paletteOpen } from "$lib/stores/command-palette.js"
import { workspaces } from "$lib/stores/workspaces.js"
import { providers } from "$lib/stores/providers.js"
import { machines } from "$lib/stores/machines.js"
import type { PaletteItem } from "$lib/stores/command-palette.js"
import * as Command from "$lib/components/ui/command/index.js"
import { LayoutDashboard, Box, Plug, Server, KeyRound } from "lucide-svelte"

// eslint-disable-next-line @typescript-eslint/no-explicit-any -- lucide-svelte types don't align with Svelte 5 Component
const CATEGORY_ICONS: Record<string, typeof LayoutDashboard> = {
  Navigation: LayoutDashboard,
  Workspaces: Box,
  Providers: Plug,
  Machines: Server,
  "SSH Keys": KeyRound,
}

// Build items list from navigation + dynamic resources
let allItems = $derived.by(() => {
  const items: PaletteItem[] = [
    {
      id: "nav-dashboard",
      label: "Dashboard",
      description: "Go to dashboard",
      category: "Navigation",
      href: "/",
    },
    {
      id: "nav-workspaces",
      label: "Workspaces",
      description: "View all workspaces",
      category: "Navigation",
      href: "/workspaces",
    },
    {
      id: "nav-new-workspace",
      label: "New Workspace",
      description: "Create a workspace",
      category: "Navigation",
      href: "/workspaces/new",
    },
    {
      id: "nav-providers",
      label: "Providers",
      description: "View all providers",
      category: "Navigation",
      href: "/providers",
    },
    {
      id: "nav-add-provider",
      label: "Add Provider",
      description: "Add a new provider",
      category: "Navigation",
      href: "/providers/add",
    },
    {
      id: "nav-machines",
      label: "Machines",
      description: "View all machines",
      category: "Navigation",
      href: "/machines",
    },
    {
      id: "nav-contexts",
      label: "Contexts",
      description: "Manage contexts",
      category: "Navigation",
      href: "/contexts",
    },
    {
      id: "nav-terminals",
      label: "Terminals",
      description: "Terminal sessions",
      category: "Navigation",
      href: "/terminals",
    },
    {
      id: "nav-ssh-keys",
      label: "SSH Keys",
      description: "Manage SSH keys",
      category: "Navigation",
      href: "/ssh-keys",
    },
    {
      id: "nav-settings",
      label: "Settings",
      description: "App settings",
      category: "Navigation",
      href: "/settings",
    },
  ]

  for (const ws of $workspaces) {
    items.push({
      id: `ws-${ws.id}`,
      label: ws.id,
      description: ws.provider?.name ?? "",
      category: "Workspaces",
      href: `/workspaces/${ws.id}`,
    })
  }

  for (const p of $providers) {
    items.push({
      id: `prov-${p.name}`,
      label: p.name,
      description: p.version ?? "",
      category: "Providers",
      href: `/providers/${p.name}`,
    })
  }

  for (const m of $machines) {
    items.push({
      id: `mach-${m.id}`,
      label: m.id,
      description: m.provider?.name ?? "",
      category: "Machines",
      href: `/machines/${m.id}`,
    })
  }

  return items
})

// Group items by category
let grouped = $derived.by(() => {
  const groups: { category: string; items: PaletteItem[] }[] = []
  const seen = new Set<string>()
  for (const item of allItems) {
    const cat = item.category ?? ""
    if (!seen.has(cat)) {
      seen.add(cat)
      groups.push({ category: cat, items: [] })
    }
    const group = groups.find((g) => g.category === cat)
    if (group) group.items.push(item)
  }
  return groups
})

function selectItem(item: PaletteItem) {
  if (item.href) goto(item.href)
  if (item.action) item.action()
  paletteOpen.set(false)
}
</script>

<Command.Dialog bind:open={$paletteOpen} title="Command Palette" description="Search for a command to run...">
  <Command.Input placeholder="Type a command or search..." />
  <Command.List>
    <Command.Empty>No results found.</Command.Empty>
    {#each grouped as group}
      <Command.Group heading={group.category}>
        {#each group.items as item (item.id)}
          {@const Icon = item.category ? CATEGORY_ICONS[item.category] : undefined}
          <Command.Item
            value="{item.label} {item.description ?? ''}"
            onSelect={() => selectItem(item)}
          >
            {#if Icon}
              <Icon class="h-4 w-4 shrink-0 text-muted-foreground" />
            {/if}
            <div class="min-w-0 flex-1">
              <div class="truncate font-medium">{item.label}</div>
              {#if item.description}
                <div class="truncate text-xs text-muted-foreground">
                  {item.description}
                </div>
              {/if}
            </div>
          </Command.Item>
        {/each}
      </Command.Group>
    {/each}
  </Command.List>
</Command.Dialog>
