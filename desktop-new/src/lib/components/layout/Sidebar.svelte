<script lang="ts">
import {
  Box,
  Layers,
  LayoutDashboard,
  KeyRound,
  Plug,
  Search,
  Server,
  Settings,
  SquareTerminal,
} from "@lucide/svelte"
import { page } from "$app/stores"
import * as Sidebar from "$lib/components/ui/sidebar/index.js"
import { workspaces } from "$lib/stores/workspaces.js"
import { providers } from "$lib/stores/providers.js"
import { machines } from "$lib/stores/machines.js"
import { contexts } from "$lib/stores/contexts.js"
import { togglePalette } from "$lib/stores/command-palette.js"
import type { Component } from "svelte"

let { terminalCount = 0 }: { terminalCount?: number } = $props()

interface NavItem {
  href: string
  label: string
  icon: Component
  badge?: number
}

let mainNav: NavItem[] = $derived([
  { href: "/", label: "Dashboard", icon: LayoutDashboard },
  {
    href: "/workspaces",
    label: "Workspaces",
    icon: Box,
    badge: $workspaces.length,
  },
  {
    href: "/providers",
    label: "Providers",
    icon: Plug,
    badge: $providers.length,
  },
  {
    href: "/machines",
    label: "Machines",
    icon: Server,
    badge: $machines.length,
  },
  {
    href: "/contexts",
    label: "Contexts",
    icon: Layers,
    badge: $contexts.length,
  },
  {
    href: "/terminals",
    label: "Terminals",
    icon: SquareTerminal,
    badge: terminalCount,
  },
  { href: "/ssh-keys", label: "SSH Keys", icon: KeyRound },
])

function isActive(href: string): boolean {
  return href === "/"
    ? $page.url.pathname === "/"
    : $page.url.pathname.startsWith(href)
}
</script>

<Sidebar.Root collapsible="icon">
  <Sidebar.Header>
    <Sidebar.Menu>
      <Sidebar.MenuItem>
        <Sidebar.MenuButton size="lg" class="pointer-events-none">
          <div class="flex aspect-square size-8 items-center justify-center rounded-lg bg-primary text-primary-foreground">
            <Box class="size-4" />
          </div>
          <div class="grid flex-1 text-left text-sm leading-tight">
            <span class="truncate font-semibold">DevPod</span>
            <span class="truncate text-xs text-muted-foreground">Desktop</span>
          </div>
        </Sidebar.MenuButton>
      </Sidebar.MenuItem>
    </Sidebar.Menu>
  </Sidebar.Header>

  <Sidebar.Content>
    <Sidebar.Group>
      <Sidebar.GroupLabel>Navigation</Sidebar.GroupLabel>
      <Sidebar.GroupContent>
        <Sidebar.Menu>
          {#each mainNav as item (item.href)}
            {@const Icon = item.icon}
            <Sidebar.MenuItem>
              <Sidebar.MenuButton isActive={isActive(item.href)} tooltipContent={item.label}>
                {#snippet child({ props })}
                  <a href={item.href} {...props}>
                    <Icon />
                    <span>{item.label}</span>
                  </a>
                {/snippet}
              </Sidebar.MenuButton>
              {#if item.badge != null && item.badge > 0}
                <Sidebar.MenuBadge>{item.badge}</Sidebar.MenuBadge>
              {/if}
            </Sidebar.MenuItem>
          {/each}
        </Sidebar.Menu>
      </Sidebar.GroupContent>
    </Sidebar.Group>
  </Sidebar.Content>

  <Sidebar.Footer>
    <Sidebar.Menu>
      <Sidebar.MenuItem>
        <Sidebar.MenuButton isActive={isActive("/settings")} tooltipContent="Settings">
          {#snippet child({ props })}
            <a href="/settings" {...props}>
              <Settings />
              <span>Settings</span>
            </a>
          {/snippet}
        </Sidebar.MenuButton>
      </Sidebar.MenuItem>
      <Sidebar.MenuItem>
        <Sidebar.MenuButton tooltipContent="Search (⌘K)" onclick={togglePalette}>
          <Search />
          <span>Search</span>
          <kbd class="ml-auto rounded border bg-muted px-1.5 py-0.5 text-xs font-mono group-data-[collapsible=icon]:hidden">⌘K</kbd>
        </Sidebar.MenuButton>
      </Sidebar.MenuItem>
    </Sidebar.Menu>
  </Sidebar.Footer>

  <Sidebar.Rail />
</Sidebar.Root>
