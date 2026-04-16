import { spawn, spawnSync } from "node:child_process"
import os from "node:os"
import path from "node:path"
import { fileURLToPath } from "node:url"

const __dirname = fileURLToPath(new URL(".", import.meta.url))

// Binary name from Cargo.toml [package] name
const application = path.resolve(
  __dirname,
  "..",
  "src-tauri",
  "target",
  "debug",
  "devpod-desktop-new",
)

let tauriDriver
let exit = false

export const config = {
  host: "127.0.0.1",
  port: 4444,
  specs: ["./test/specs/**/*.js"],
  maxInstances: 1,
  capabilities: [
    {
      maxInstances: 1,
      "tauri:options": {
        application,
      },
    },
  ],
  reporters: ["spec"],
  framework: "mocha",
  mochaOpts: {
    ui: "bdd",
    timeout: 120000,
  },

  // Build the Tauri app in debug mode before running tests
  onPrepare: () => {
    spawnSync(
      "npm",
      ["run", "tauri", "build", "--", "--debug", "--no-bundle"],
      {
        cwd: path.resolve(__dirname, ".."),
        stdio: "inherit",
        shell: true,
      },
    )
  },

  // Start tauri-driver before each session
  beforeSession: () => {
    tauriDriver = spawn(
      path.resolve(os.homedir(), ".cargo", "bin", "tauri-driver"),
      [],
      { stdio: [null, process.stdout, process.stderr] },
    )

    tauriDriver.on("error", (error) => {
      console.error("tauri-driver error:", error)
      process.exit(1)
    })

    tauriDriver.on("exit", (code) => {
      if (!exit) {
        console.error("tauri-driver exited with code:", code)
        process.exit(1)
      }
    })
  },

  // Clean up tauri-driver after each session
  afterSession: () => {
    closeTauriDriver()
  },
}

function closeTauriDriver() {
  exit = true
  tauriDriver?.kill()
}

function onShutdown(fn) {
  const cleanup = () => {
    try {
      fn()
    } finally {
      process.exit()
    }
  }

  process.on("exit", cleanup)
  process.on("SIGINT", cleanup)
  process.on("SIGTERM", cleanup)
  process.on("SIGHUP", cleanup)
  process.on("SIGBREAK", cleanup)
}

onShutdown(() => {
  closeTauriDriver()
})
