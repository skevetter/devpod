import fs from "fs"
import path from "path"
import { glob } from "glob"
import { fileURLToPath } from "url"

const __dirname = path.dirname(fileURLToPath(import.meta.url))

async function renameArtifacts() {
  const bundleDir = path.join(__dirname, "../src-tauri/target/release/bundle")

  if (!fs.existsSync(bundleDir)) {
    console.log("Bundle directory not found, skipping rename")
    return
  }

  const patterns = [
    "**/*_*_*.deb",
    "**/*_*_*.AppImage",
    "**/*-*-*.rpm",
    "**/*_*_*.exe",
    "**/*_*_*.msi",
    "**/*_*_*.dmg",
    "**/*_*.app.tar.gz",
  ]

  const platformMap = {
    ".deb": "linux",
    ".AppImage": "linux",
    ".rpm": "linux",
    ".exe": "windows",
    ".msi": "windows",
    ".dmg": "macos",
    ".app.tar.gz": "macos",
  }

  for (const pattern of patterns) {
    const files = await glob(pattern, { cwd: bundleDir })

    for (const file of files) {
      const oldPath = path.join(bundleDir, file)
      const fileName = path.basename(file)

      // Extract components from filename
      let platform = ""
      let arch = ""

      // Determine platform from extension
      for (const [ext, plat] of Object.entries(platformMap)) {
        if (fileName.endsWith(ext)) {
          platform = plat
          break
        }
      }

      // Extract architecture
      if (fileName.includes("amd64")) arch = "x86_64"
      else if (fileName.includes("x64")) arch = "x64"
      else if (fileName.includes("aarch64")) arch = "aarch64"

      // Generate new filename based on format
      let newFileName = fileName
      if (platform && arch) {
        if (fileName.endsWith(".app.tar.gz")) {
          newFileName = `DevPod_${platform}_${arch}.app.tar.gz`
        } else if (fileName.endsWith(".msi")) {
          newFileName = `DevPod_${platform}_${arch}_en-US.msi`
        } else if (fileName.endsWith(".exe")) {
          newFileName = `DevPod_${platform}_${arch}-setup.exe`
        } else {
          const ext = path.extname(fileName)
          newFileName = `DevPod_${platform}_${arch}${ext}`
        }
      }

      const newPath = path.join(path.dirname(oldPath), newFileName)

      if (oldPath !== newPath && fs.existsSync(oldPath)) {
        fs.renameSync(oldPath, newPath)
        console.log(`Renamed: ${fileName} → ${newFileName}`)

        // Rename signature file if exists
        if (fs.existsSync(oldPath + ".sig")) {
          fs.renameSync(oldPath + ".sig", newPath + ".sig")
          console.log(`Renamed: ${fileName}.sig → ${newFileName}.sig`)
        }
      }
    }
  }
}

renameArtifacts().catch(console.error)
