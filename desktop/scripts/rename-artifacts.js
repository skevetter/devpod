import { existsSync, renameSync } from "fs"
import { join, basename, extname, dirname } from "path"
import { glob } from "glob"

async function renameArtifacts() {
  const bundleDir = join(__dirname, "../src-tauri/target/release/bundle")

  if (!existsSync(bundleDir)) {
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
      const oldPath = join(bundleDir, file)
      const fileName = basename(file)

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
          const ext = extname(fileName)
          newFileName = `DevPod_${platform}_${arch}${ext}`
        }
      }

      const newPath = join(dirname(oldPath), newFileName)

      if (oldPath !== newPath && existsSync(oldPath)) {
        renameSync(oldPath, newPath)
        console.log(`renamed ${fileName} → ${newFileName}`)

        // Rename signature file if exists
        if (existsSync(oldPath + ".sig")) {
          renameSync(oldPath + ".sig", newPath + ".sig")
          console.log(`renamed ${fileName}.sig → ${newFileName}.sig`)
        }
      }
    }
  }
}

renameArtifacts().catch(console.error)
