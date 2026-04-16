/**
 * IPC bridge — re-exports invoke/listen from Tauri when available,
 * falls back to mock implementations for browser-only development.
 */

import { isTauri } from "./mock.js"

// Dynamic re-export based on runtime environment
let _invoke: typeof import("@tauri-apps/api/core").invoke
let _listen: typeof import("@tauri-apps/api/event").listen

if (isTauri()) {
  // Running inside Tauri webview — use real IPC
  const core = await import("@tauri-apps/api/core")
  const event = await import("@tauri-apps/api/event")
  _invoke = core.invoke
  _listen = event.listen
} else {
  // Running in plain browser — use mocks
  console.info(
    "%c[DevPod] Running in browser mock mode",
    "color: #f59e0b; font-weight: bold",
  )
  const mock = await import("./mock.js")
  _invoke = mock.invoke as typeof _invoke
  _listen = mock.listen as typeof _listen
}

export const invoke = _invoke
export const listen = _listen
