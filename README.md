<br>
<a href="https://www.devpod.sh">
  <picture width="500">
    <source media="(prefers-color-scheme: dark)" srcset="docs/static/media/devpod_dark.png">
    <img alt="DevPod wordmark" width="500" src="docs/static/media/devpod.png">
  </picture>
</a>

### **[Website](https://www.devpod.sh)** • **[Quickstart](https://www.devpod.sh/docs/getting-started/install)** • **[Documentation](https://www.devpod.sh/docs/what-is-devpod)** • **[GitHub](https://github.com/skevetter/devpod)**

[![Open in DevPod!](https://devpod.sh/assets/open-in-devpod.svg)](https://devpod.sh/open#https://github.com/skevetter/devpod)

DevPod is a client-only tool to create reproducible developer environments based on a [devcontainer.json](https://containers.dev/) on any backend. Each developer environment runs in a container and is specified through a [devcontainer.json](https://containers.dev/). Through DevPod providers, these environments can be created on any backend, such as the local computer, a Kubernetes cluster, any reachable remote machine, or in a VM in the cloud.

<table align="center" width="80%" cellspacing="20">
<tr>
<td width="33%" align="center" valign="top">
<img src="https://cdn.prod.website-files.com/645b6806227d4a212e2d01ca/645e0b378e0d30107e68e70d_icons8-open-source%201.svg" loading="lazy" width="64" height="64" alt="" style="margin-bottom: 16px;">
<h3 style="margin: 12px 0; font-weight: 600;">Open Source</h3>
<p style="line-height: 1.6;">No vendor lock-in. 100% free and open source built by developers for developers.</p>
</td>
<td width="33%" align="center" valign="top">
<img src="https://cdn.prod.website-files.com/645b6806227d4a212e2d01ca/645e0b37fe80bc64fc19af0d_focus%201.svg" loading="lazy" width="64" height="64" alt="" style="margin-bottom: 16px;">
<h3 style="margin: 12px 0; font-weight: 600;">Client Only</h3>
<p style="line-height: 1.6;">No server side setup needed. Download the desktop app or the CLI to get started.</p>
</td>
<td width="33%" align="center" valign="top">
<img src="https://cdn.prod.website-files.com/645b6806227d4a212e2d01ca/645e0b38a712b0144abc1e4b_window-code%201.svg" loading="lazy" width="64" height="64" alt="" style="margin-bottom: 16px;">
<h3 style="margin: 12px 0; font-weight: 600;">Unopinionated</h3>
<p style="line-height: 1.6;">Repeatable dev environment for any infra, any IDE, and any programming language.</p>
</td>
</tr>
</table>

You can think of DevPod as the glue that connects your local IDE to a machine where you want to develop. So depending on the requirements of your project, you can either create a workspace locally on the computer, on a beefy cloud machine with many GPUs, or a spare remote computer. Within DevPod, every workspace is managed the same way, which also makes it easy to switch between workspaces that might be hosted somewhere else.

![DevPod Flow](docs/static/media/devpod-flow.gif)

<h2 align="center">Downloads</h2>

<p>
DevPod is available as both a desktop application with a graphical interface and a command-line tool.
Take a look at the <a href="https://devpod.sh/docs/getting-started/install">DevPod Docs</a> for installation instructions and more information.
</p>

<table align="center" width="80%">
<tr>
<td width="50%" valign="top">

<h3>Desktop Application</h3>

<table>
<tr>
<th>Platform</th>
<th>Architecture</th>
<th>Download</th>
</tr>
<tr>
<td><b>macOS</b></td>
<td>Apple Silicon (ARM64)</td>
<td><a href="https://github.com/skevetter/devpod/releases/latest/download/DevPod_darwin_aarch64.dmg"><img src="https://img.shields.io/badge/Download-DMG-blue?style=flat-square&logo=apple" alt="macOS ARM64"></a></td>
</tr>
<tr>
<td><b>macOS</b></td>
<td>Intel (x64)</td>
<td><a href="https://github.com/skevetter/devpod/releases/latest/download/DevPod_darwin_x64.dmg"><img src="https://img.shields.io/badge/Download-DMG-blue?style=flat-square&logo=apple" alt="macOS x64"></a></td>
</tr>
<tr>
<td><b>Windows</b></td>
<td>x64</td>
<td><a href="https://github.com/skevetter/devpod/releases/latest/download/DevPod_windows_x64.msi"><img src="https://img.shields.io/badge/Download-MSI-blue?style=flat-square&logo=windows" alt="Windows MSI"></a></td>
</tr>
<tr>
<td><b>Windows</b></td>
<td>x64 (Portable)</td>
<td><a href="https://github.com/skevetter/devpod/releases/latest/download/DevPod_windows_x64.exe"><img src="https://img.shields.io/badge/Download-EXE-blue?style=flat-square&logo=windows" alt="Windows EXE"></a></td>
</tr>
<tr>
<td><b>Linux</b></td>
<td>x64 (AppImage)</td>
<td><a href="https://github.com/skevetter/devpod/releases/latest/download/DevPod_linux_amd64.AppImage"><img src="https://img.shields.io/badge/Download-AppImage-blue?style=flat-square&logo=linux" alt="Linux AppImage"></a></td>
</tr>
<tr>
<td><b>Linux</b></td>
<td>x64 (Debian/Ubuntu)</td>
<td><a href="https://github.com/skevetter/devpod/releases/latest/download/DevPod_linux_amd64.deb"><img src="https://img.shields.io/badge/Download-DEB-blue?style=flat-square&logo=debian" alt="Linux DEB"></a></td>
</tr>
<tr>
<td><b>Linux</b></td>
<td>x64 (RPM)</td>
<td><a href="https://github.com/skevetter/devpod/releases/latest/download/DevPod_linux_x86_64.rpm"><img src="https://img.shields.io/badge/Download-RPM-blue?style=flat-square&logo=redhat" alt="Linux RPM"></a></td>
</tr>
<tr>
<td><b>Linux</b></td>
<td>x64 (Flatpak)</td>
<td><a href="https://github.com/skevetter/devpod/releases/latest/download/DevPod.flatpak"><img src="https://img.shields.io/badge/Download-Flatpak-blue?style=flat-square&logo=flatpak" alt="Linux Flatpak"></a></td>
</tr>
</table>

</td>
<td width="50%" valign="top">

<h3>CLI</h3>

<table>
<tr>
<th>Platform</th>
<th>Architecture</th>
<th>Download</th>
</tr>
<tr>
<td><b>macOS</b></td>
<td>Apple Silicon (ARM64)</td>
<td><a href="https://github.com/skevetter/devpod/releases/latest/download/devpod-darwin-arm64"><img src="https://img.shields.io/badge/Download-Binary-green?style=flat-square&logo=apple" alt="macOS ARM64 CLI"></a></td>
</tr>
<tr>
<td><b>macOS</b></td>
<td>Intel (x64)</td>
<td><a href="https://github.com/skevetter/devpod/releases/latest/download/devpod-darwin-amd64"><img src="https://img.shields.io/badge/Download-Binary-green?style=flat-square&logo=apple" alt="macOS x64 CLI"></a></td>
</tr>
<tr>
<td><b>Linux</b></td>
<td>x64</td>
<td><a href="https://github.com/skevetter/devpod/releases/latest/download/devpod-linux-amd64"><img src="https://img.shields.io/badge/Download-Binary-green?style=flat-square&logo=linux" alt="Linux x64 CLI"></a></td>
</tr>
<tr>
<td><b>Linux</b></td>
<td>ARM64</td>
<td><a href="https://github.com/skevetter/devpod/releases/latest/download/devpod-linux-arm64"><img src="https://img.shields.io/badge/Download-Binary-green?style=flat-square&logo=linux" alt="Linux ARM64 CLI"></a></td>
</tr>
<tr>
<td><b>Windows</b></td>
<td>x64</td>
<td><a href="https://github.com/skevetter/devpod/releases/latest/download/devpod-windows-amd64.exe"><img src="https://img.shields.io/badge/Download-EXE-green?style=flat-square&logo=windows" alt="Windows x64 CLI"></a></td>
</tr>
<tr>
<td><b>Windows</b></td>
<td>ARM64</td>
<td><a href="https://github.com/skevetter/devpod/releases/latest/download/devpod-windows-arm64.exe"><img src="https://img.shields.io/badge/Download-EXE-green?style=flat-square&logo=windows" alt="Windows ARM64 CLI"></a></td>
</tr>
</table>

</td>
</tr>
</table>

<h2 align="center">Why DevPod?</h2>

<p>
DevPod reuses the open <a href="https://containers.dev/">DevContainer standard</a> (used by GitHub Codespaces and VSCode DevContainers) to create a consistent developer experience no matter what backend you want to use.
</p>

<table>
<tr>
<td width="50%" valign="top">
<b>Cost savings</b><br>
DevPod is usually around 5-10 times cheaper than existing services with comparable feature sets because it uses bare virtual machines in any cloud and shuts down unused virtual machines automatically.
</td>
<td width="50%" valign="top">
<b>No vendor lock-in</b><br>
Choose whatever cloud provider suits you best, be it the cheapest one or the most powerful, DevPod supports all cloud providers. If you are tired of using a provider, change it with a single command.
</td>
</tr>
<tr>
<td width="50%" valign="top">
<b>Local development</b><br>
You get the same developer experience also locally, so you don't need to rely on a cloud provider at all.
</td>
<td width="50%" valign="top">
<b>Cross IDE support</b><br>
VSCode and the full JetBrains suite is supported, all others can be connected through simple ssh.
</td>
</tr>
<tr>
<td width="50%" valign="top">
<b>Client-only</b><br>
No need to install a server backend, DevPod runs only on your computer.
</td>
<td width="50%" valign="top">
<b>Open-Source</b><br>
DevPod is 100% open-source and extensible. A provider doesn't exist? Just create your own.
</td>
</tr>
<tr>
<td width="50%" valign="top">
<b>Rich feature set</b><br>
DevPod already supports prebuilds, auto inactivity shutdown, git & docker credentials sync, and many more features to come.
</td>
<td width="50%" valign="top">
<b>Desktop App</b><br>
DevPod comes with an easy-to-use desktop application that abstracts all the complexity away. If you want to build your own integration, DevPod offers a feature-rich CLI as well.
</td>
</tr>
</table>
