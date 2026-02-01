# Contributing to DevPod

Thank you for your interest in contributing to DevPod! This guide will help you get started with development.

## Development Setup

### Prerequisites

**CLI Development:**

- [Go 1.25+](https://go.dev/doc/install)
- [Task](https://taskfile.dev/) (optional, but recommended for running tasks)

**Desktop Application Development:**

- [NodeJS + yarn](https://nodejs.org/en/)
- [Rust](https://www.rust-lang.org/tools/install)
- [Go 1.20+](https://go.dev/doc/install)

**Linux Desktop Dependencies:**

```bash
sudo apt-get install libappindicator3-1 libgdk-pixbuf2.0-0 libbsd0 libxdmcp6 \
  libwmf-0.2-7 libwmf-0.2-7-gtk libgtk-3-0 libwmf-dev libwebkit2gtk-4.0-37 \
  librust-openssl-sys-dev librust-glib-sys-dev

sudo apt-get install -y libgtk-3-dev libwebkit2gtk-4.0-dev \
  libayatana-appindicator3-dev librsvg2-dev
```

### Initial Setup

1. Clone the repository:
   ```bash
   git clone https://github.com/skevetter/devpod.git
   cd devpod
   ```

2. If you want to change DevPod agent code:
   - Exchange the URL in [DefaultAgentDownloadURL](./pkg/agent/agent.go) with a custom public repository release
   - Build devpod via: `task cli:build:dev`
   - Upload `dist/devpod-dev_linux_amd64_v1/devpod-linux-amd64` and ARM64 variant to your public repository release assets

## Building from Source

### CLI

**Using Task (recommended):**

```bash
# Build CLI for development
task cli:build:dev

# Build CLI for production
task cli:build

# Build with Pro features
task cli:build:dev:pro
```

**Using Go directly:**

```bash
CGO_ENABLED=0 go build -ldflags "-s -w" -o devpod
```

The binary will be output as `devpod` in the current directory.

### Desktop Application

**Using Task (recommended):**

```bash
# Setup Tauri environment (first time only)
task desktop:tauri:setup

# Run in development mode
task desktop:tauri:dev

# Build the application
task desktop:tauri:build-app
```

**Manual build:**

```bash
cd desktop
yarn install --frozen-lockfile
yarn desktop:build:dev
```

The application will be in `desktop/src-tauri/target/release`

## Development Workflow

### CLI Development

```bash
# Tidy dependencies
task cli:tidy

# Run linters
task cli:lint

# Run unit tests
task cli:test

# Build for development
task cli:build:dev
```

### Desktop Development

```bash
# Install dependencies
cd desktop
yarn install --frozen-lockfile

# Check code quality
task desktop:check

# Run linters
yarn lint:ci

# Check formatting
yarn format:check

# Type check
yarn types:check

# Generate TypeScript types from Rust
task desktop:tauri:generate-types

# Update dependencies
task desktop:update-deps
```

### E2E Testing

```bash
# Build for e2e tests
task cli:test:e2e:build

# Run all e2e tests
task cli:test:e2e

# Run specific test suite
task cli:test:e2e:suite -- "suite-name"

# Run focused tests
task cli:test:e2e:focus -- "test-pattern"

# Setup kind cluster for testing
task cli:test:e2e:kind:setup

# Teardown kind cluster
task cli:test:e2e:kind:teardown
```

### gRPC Development

If you need to modify the gRPC tunnel code:

```bash
task cli:build:grpc
```

This requires:
- `protobuf-compiler` (install via `sudo apt install protobuf-compiler`)
- Go protobuf plugins (installed automatically by the task)

## Testing Your Changes

### Quick Start

1. Build DevPod:
   ```bash
   task cli:build:dev
   ```

2. Add a provider:
   ```bash
   ./dist/devpod-dev_linux_amd64_v1/devpod-linux-amd64 provider add docker
   ```

3. Configure the provider:
   ```bash
   ./dist/devpod-dev_linux_amd64_v1/devpod-linux-amd64 provider use docker
   ```

4. Start a workspace:
   ```bash
   ./dist/devpod-dev_linux_amd64_v1/devpod-linux-amd64 up examples/simple
   ```

### Using Act for Local CI Testing

```bash
# Build UI using act
task desktop:act:build:ui

# Build desktop app using act
task desktop:act:build:app

# Build flatpak using act
task desktop:act:build:flatpak

# Run e2e tests with focus
task cli:test:e2e:act:focus -- "test-pattern"
```

## Developing Providers

Read [the docs](https://devpod.sh/docs/developing-providers/quickstart) for an introduction to developing your own providers.

### Publishing Your Provider

Once your provider is ready:

1. Update `community.yaml` with your provider information
2. Update `docs/pages/managing-providers/add-provider.mdx` with documentation

This will feature your provider in both the documentation and the UI.

## Desktop Deep Links

DevPod Desktop can handle deep links to perform various actions.

**URL Scheme:**

```
devpod://command?param1=value1&param2=value2
```

### Open Workspace

Open a workspace based on a source (similar to `devpod up`, but shareable):

```
devpod://open?source=<url-encoded-source>&workspace=<name>&provider=<provider>&ide=<ide>
```

**Parameters:**

- `source` (required): URL-encoded workspace source
- `workspace` (optional): Workspace name
- `provider` (optional): Provider to use
- `ide` (optional): IDE to open

**Example:**

```
devpod://open?source=https%3A%2F%2Fgithub.com%2Fuser%2Frepo&workspace=my-workspace&provider=docker&ide=vscode
```

### Import Workspace

Import a remote DevPod.Pro workspace into your local client:

```
devpod://import?workspace_id=<id>&workspace_uid=<uid>&devpod_pro_host=<host>&options=<options>
```

**Parameters:**
- `workspace_id` (required): Workspace ID
- `workspace_uid` (required): Workspace UID
- `devpod_pro_host` (required): DevPod Pro host URL
- `options` (optional): Additional options

## Useful Task Commands

```bash
# View all available tasks
task --list

# CLI tasks
task cli:build              # Build CLI for production
task cli:build:dev          # Build CLI for development
task cli:lint               # Run linters
task cli:test               # Run unit tests
task cli:tidy               # Tidy go.mod and go.sum

# Desktop tasks
task desktop:build          # Build desktop application
task desktop:check          # Check code quality
task desktop:tauri:dev      # Run in development mode
task desktop:tauri:setup    # Setup Tauri environment

# E2E testing
task cli:test:e2e           # Run all e2e tests
task cli:test:e2e:suite     # Run specific test suite
task cli:test:e2e:focus     # Run focused tests
```

## Repository Structure

- `/cmd` - CLI command implementations
- `/pkg` - Core packages and libraries
- `/desktop` - Desktop application (Tauri + React)
- `/e2e` - End-to-end tests
- `/docs` - Documentation website
- `/examples` - Example devcontainer configurations
- `/hack` - Build and development scripts

## Getting Help

- [Documentation](https://devpod.sh/docs)
- [GitHub Issues](https://github.com/skevetter/devpod/issues)
- [GitHub Discussions](https://github.com/skevetter/devpod/discussions)

## Code of Conduct

Please be [respectful and constructive](https://mikemcquaid.com/open-source-maintainers-owe-you-nothing/) in all interactions with the community.
