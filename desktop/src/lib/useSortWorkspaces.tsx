import { TWorkspace } from "@/types"
import { useMemo } from "react"
import { ProWorkspaceInstance } from "@/contexts"
import { getLastActivity } from "@/lib/pro"
import { ESortWorkspaceMode, sortWorkspaces } from "./sortWorkspaceConstants"

export function useSortWorkspaces(
  workspaces: readonly TWorkspace[] | undefined,
  sortMode: ESortWorkspaceMode | undefined
) {
  return useMemo(() => {
    if (!workspaces) {
      return undefined
    }

    const sortables = workspaces.map((workspace) => ({
      original: workspace,
      created: new Date(workspace.creationTimestamp).getTime(),
      used: new Date(workspace.lastUsed).getTime(),
    }))

    return sortWorkspaces(sortables, sortMode)
  }, [workspaces, sortMode])
}

export function useSortProWorkspaces(
  workspaces: readonly ProWorkspaceInstance[] | undefined,
  sortMode: ESortWorkspaceMode | undefined
) {
  return useMemo(() => {
    if (!workspaces) {
      return undefined
    }

    const sortables = workspaces.map((workspace) => ({
      original: workspace,
      created: new Date(workspace.metadata?.creationTimestamp ?? 0).getTime(),
      used: getLastActivity(workspace)?.getTime() ?? 0,
    }))

    return sortWorkspaces(sortables, sortMode)
  }, [workspaces, sortMode])
}
