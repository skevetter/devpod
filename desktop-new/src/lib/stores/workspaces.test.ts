import { get } from "svelte/store"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import {
  mockInvoke,
  mockListen,
  resetTauriMocks,
} from "$lib/__mocks__/tauri.js"

import {
  destroyWorkspaces,
  initWorkspaces,
  workspaces,
  workspacesLoading,
} from "./workspaces.js"

describe("workspaces store", () => {
  beforeEach(() => {
    vi.useFakeTimers()
    resetTauriMocks()
    workspaces.set([])
    workspacesLoading.set(true)
  })

  afterEach(() => {
    destroyWorkspaces()
    vi.useRealTimers()
  })

  it("loads workspaces on init", async () => {
    const mockWorkspaces = [
      { id: "ws-1", source: { gitRepository: "github.com/org/repo" } },
      { id: "ws-2", source: { image: "ubuntu:latest" } },
    ]
    mockInvoke.mockImplementation((cmd: string) => {
      if (cmd === "workspace_list") return Promise.resolve(mockWorkspaces)
      if (cmd === "workspace_status")
        return Promise.resolve('{"state":"Stopped"}')
      return Promise.resolve(undefined)
    })

    await initWorkspaces()

    expect(get(workspacesLoading)).toBe(false)
    const current = get(workspaces)
    expect(current).toHaveLength(2)
    expect(current[0].id).toBe("ws-1")
    expect(current[1].id).toBe("ws-2")
  })

  it("sets loading false even on error", async () => {
    mockInvoke.mockRejectedValue(new Error("Tauri not available"))

    await initWorkspaces()

    expect(get(workspacesLoading)).toBe(false)
    expect(get(workspaces)).toEqual([])
  })

  it("subscribes to workspace change events", async () => {
    mockInvoke.mockImplementation((cmd: string) => {
      if (cmd === "workspace_list") return Promise.resolve([])
      if (cmd === "workspace_status")
        return Promise.resolve('{"state":"Stopped"}')
      return Promise.resolve(undefined)
    })

    await initWorkspaces()

    expect(mockListen).toHaveBeenCalledWith(
      "workspaces-changed",
      expect.any(Function),
    )
  })

  it("fetches status for each workspace after loading", async () => {
    const mockWorkspaces = [{ id: "ws-1" }, { id: "ws-2" }]
    mockInvoke.mockImplementation(
      (cmd: string, args?: Record<string, unknown>) => {
        if (cmd === "workspace_list") return Promise.resolve(mockWorkspaces)
        if (cmd === "workspace_status") {
          const wsId = args?.workspaceId as string
          if (wsId === "ws-1") return Promise.resolve('{"state":"Running"}')
          if (wsId === "ws-2") return Promise.resolve('{"state":"Stopped"}')
        }
        return Promise.resolve(undefined)
      },
    )

    await initWorkspaces()
    // Let status fetches resolve
    await vi.waitFor(() => {
      const current = get(workspaces)
      expect(current.find((w) => w.id === "ws-1")?.status).toBe("Running")
      expect(current.find((w) => w.id === "ws-2")?.status).toBe("Stopped")
    })
  })

  it("handles status fetch failures gracefully", async () => {
    const mockWorkspaces = [{ id: "ws-1" }]
    mockInvoke.mockImplementation((cmd: string) => {
      if (cmd === "workspace_list") return Promise.resolve(mockWorkspaces)
      if (cmd === "workspace_status")
        return Promise.reject(new Error("timeout"))
      return Promise.resolve(undefined)
    })

    await initWorkspaces()
    // Flush pending promises
    await vi.advanceTimersByTimeAsync(0)

    const current = get(workspaces)
    expect(current).toHaveLength(1)
    // Status should remain undefined since fetch failed
    expect(current[0].status).toBeUndefined()
  })

  it("destroyWorkspaces cleans up listener", async () => {
    const mockUnlisten = vi.fn()
    mockListen.mockResolvedValue(mockUnlisten)
    mockInvoke.mockImplementation((cmd: string) => {
      if (cmd === "workspace_list") return Promise.resolve([])
      return Promise.resolve(undefined)
    })

    await initWorkspaces()
    destroyWorkspaces()

    expect(mockUnlisten).toHaveBeenCalled()
  })

  it("polls statuses every 10 seconds", async () => {
    const mockWorkspaces = [{ id: "ws-1" }]
    let statusCallCount = 0
    mockInvoke.mockImplementation((cmd: string) => {
      if (cmd === "workspace_list") return Promise.resolve(mockWorkspaces)
      if (cmd === "workspace_status") {
        statusCallCount++
        return Promise.resolve('{"state":"Running"}')
      }
      return Promise.resolve(undefined)
    })

    await initWorkspaces()
    // Initial fetch
    const initialCalls = statusCallCount

    // Advance past one poll interval
    vi.advanceTimersByTime(10_000)
    await vi.waitFor(() => {
      expect(statusCallCount).toBeGreaterThan(initialCalls)
    })
  })

  it("destroyWorkspaces stops polling", async () => {
    mockInvoke.mockImplementation((cmd: string) => {
      if (cmd === "workspace_list") return Promise.resolve([{ id: "ws-1" }])
      if (cmd === "workspace_status")
        return Promise.resolve('{"state":"Stopped"}')
      return Promise.resolve(undefined)
    })

    await initWorkspaces()
    destroyWorkspaces()

    const callsBefore = mockInvoke.mock.calls.filter(
      (c: string[]) => c[0] === "workspace_status",
    ).length

    vi.advanceTimersByTime(30_000)

    const callsAfter = mockInvoke.mock.calls.filter(
      (c: string[]) => c[0] === "workspace_status",
    ).length
    expect(callsAfter).toBe(callsBefore)
  })
})
