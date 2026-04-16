import { vi } from "vitest"

/**
 * Mock for @tauri-apps/api/core invoke function.
 * Tests can configure responses via mockInvoke.mockImplementation()
 */
export const mockInvoke = vi.fn()

/**
 * Mock for @tauri-apps/api/event listen function.
 * Returns a no-op unlisten by default.
 */
export const mockListen = vi.fn().mockResolvedValue(() => {})

vi.mock("@tauri-apps/api/core", () => ({
  invoke: mockInvoke,
}))

vi.mock("@tauri-apps/api/event", () => ({
  listen: mockListen,
}))

// Also mock the bridge module which commands.ts/terminal.ts/events.ts import from
vi.mock("$lib/ipc/bridge", () => ({
  invoke: mockInvoke,
  listen: mockListen,
}))

export function resetTauriMocks() {
  mockInvoke.mockReset()
  mockListen.mockReset().mockResolvedValue(() => {})
}
