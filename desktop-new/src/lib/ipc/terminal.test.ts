import { beforeEach, describe, expect, it } from "vitest"
import { mockInvoke, resetTauriMocks } from "$lib/__mocks__/tauri.js"

// Import after mocks are set up
import {
  terminalClose,
  terminalCreate,
  terminalCreateSsh,
  terminalListSessions,
  terminalResize,
  terminalWrite,
} from "./terminal.js"

describe("terminal IPC commands", () => {
  beforeEach(() => {
    resetTauriMocks()
  })

  it("terminalCreate sends cols and rows", async () => {
    mockInvoke.mockResolvedValue("session-abc")
    const id = await terminalCreate(80, 24)
    expect(mockInvoke).toHaveBeenCalledWith("terminal_create", {
      cols: 80,
      rows: 24,
    })
    expect(id).toBe("session-abc")
  })

  it("terminalCreateSsh sends workspaceId, cols, rows", async () => {
    mockInvoke.mockResolvedValue("ssh-session-1")
    const id = await terminalCreateSsh("my-workspace", 120, 40)
    expect(mockInvoke).toHaveBeenCalledWith("terminal_create_ssh", {
      workspaceId: "my-workspace",
      cols: 120,
      rows: 40,
    })
    expect(id).toBe("ssh-session-1")
  })

  it("terminalWrite sends sessionId and data array", async () => {
    mockInvoke.mockResolvedValue(undefined)
    await terminalWrite("session-abc", [72, 101, 108, 108, 111])
    expect(mockInvoke).toHaveBeenCalledWith("terminal_write", {
      sessionId: "session-abc",
      data: [72, 101, 108, 108, 111],
    })
  })

  it("terminalResize sends sessionId, cols, rows", async () => {
    mockInvoke.mockResolvedValue(undefined)
    await terminalResize("session-abc", 100, 50)
    expect(mockInvoke).toHaveBeenCalledWith("terminal_resize", {
      sessionId: "session-abc",
      cols: 100,
      rows: 50,
    })
  })

  it("terminalClose sends sessionId", async () => {
    mockInvoke.mockResolvedValue(undefined)
    await terminalClose("session-abc")
    expect(mockInvoke).toHaveBeenCalledWith("terminal_close", {
      sessionId: "session-abc",
    })
  })

  it("terminalListSessions returns session id array", async () => {
    mockInvoke.mockResolvedValue(["s1", "s2", "s3"])
    const sessions = await terminalListSessions()
    expect(mockInvoke).toHaveBeenCalledWith("terminal_list")
    expect(sessions).toEqual(["s1", "s2", "s3"])
  })
})
