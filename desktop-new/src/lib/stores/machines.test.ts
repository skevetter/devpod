import { get } from "svelte/store"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import {
  mockInvoke,
  mockListen,
  resetTauriMocks,
} from "$lib/__mocks__/tauri.js"

import {
  destroyMachines,
  initMachines,
  machines,
  machinesLoading,
} from "./machines.js"

describe("machines store", () => {
  beforeEach(() => {
    vi.useFakeTimers()
    resetTauriMocks()
    machines.set([])
    machinesLoading.set(true)
  })

  afterEach(() => {
    destroyMachines()
    vi.useRealTimers()
  })

  it("loads machines on init", async () => {
    const mockMachines = [
      { id: "m1", provider: { name: "docker" } },
      { id: "m2", provider: { name: "aws" } },
    ]
    mockInvoke.mockImplementation((cmd: string) => {
      if (cmd === "machine_list") return Promise.resolve(mockMachines)
      if (cmd === "machine_status")
        return Promise.resolve('{"state":"Running"}')
      return Promise.resolve(undefined)
    })

    await initMachines()

    expect(get(machinesLoading)).toBe(false)
    expect(get(machines)).toHaveLength(2)
    expect(get(machines)[0].id).toBe("m1")
  })

  it("sets loading false on error", async () => {
    mockInvoke.mockRejectedValue(new Error("not available"))

    await initMachines()

    expect(get(machinesLoading)).toBe(false)
    expect(get(machines)).toHaveLength(0)
  })

  it("fetches status for each machine and merges into store", async () => {
    const mockMachines = [{ id: "m1" }, { id: "m2" }]
    mockInvoke.mockImplementation(
      (cmd: string, args?: Record<string, unknown>) => {
        if (cmd === "machine_list") return Promise.resolve(mockMachines)
        if (cmd === "machine_status") {
          const id = args?.id as string
          if (id === "m1") return Promise.resolve('{"state":"Running"}')
          if (id === "m2") return Promise.resolve('{"state":"Stopped"}')
        }
        return Promise.resolve(undefined)
      },
    )

    await initMachines()
    await vi.waitFor(() => {
      const current = get(machines)
      expect(current.find((m) => m.id === "m1")?.status).toBe("Running")
      expect(current.find((m) => m.id === "m2")?.status).toBe("Stopped")
    })
  })

  it("polls statuses every 10 seconds", async () => {
    const mockMachines = [{ id: "m1" }]
    let statusCallCount = 0
    mockInvoke.mockImplementation((cmd: string) => {
      if (cmd === "machine_list") return Promise.resolve(mockMachines)
      if (cmd === "machine_status") {
        statusCallCount++
        return Promise.resolve('{"state":"Running"}')
      }
      return Promise.resolve(undefined)
    })

    await initMachines()
    const initialCalls = statusCallCount

    vi.advanceTimersByTime(10_000)
    await vi.waitFor(() => {
      expect(statusCallCount).toBeGreaterThan(initialCalls)
    })
  })

  it("destroyMachines stops polling and cleans up listener", async () => {
    const mockUnlisten = vi.fn()
    mockListen.mockResolvedValue(mockUnlisten)
    mockInvoke.mockImplementation((cmd: string) => {
      if (cmd === "machine_list") return Promise.resolve([{ id: "m1" }])
      if (cmd === "machine_status")
        return Promise.resolve('{"state":"Running"}')
      return Promise.resolve(undefined)
    })

    await initMachines()
    destroyMachines()

    expect(mockUnlisten).toHaveBeenCalled()

    const callsBefore = mockInvoke.mock.calls.filter(
      (c: string[]) => c[0] === "machine_status",
    ).length

    vi.advanceTimersByTime(30_000)

    const callsAfter = mockInvoke.mock.calls.filter(
      (c: string[]) => c[0] === "machine_status",
    ).length
    expect(callsAfter).toBe(callsBefore)
  })

  it("handles status fetch failures gracefully", async () => {
    const mockMachines = [{ id: "m1" }]
    mockInvoke.mockImplementation((cmd: string) => {
      if (cmd === "machine_list") return Promise.resolve(mockMachines)
      if (cmd === "machine_status") return Promise.reject(new Error("timeout"))
      return Promise.resolve(undefined)
    })

    await initMachines()
    await vi.advanceTimersByTimeAsync(0)

    const current = get(machines)
    expect(current).toHaveLength(1)
    expect(current[0].status).toBeUndefined()
  })
})
