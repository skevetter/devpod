# Podman Provider Deep Dive Analysis

## Current State

### Existing Podman Support
DevPod already has **partial Podman support** through the Docker provider:

1. **Detection**: `DockerHelper.IsPodman()` checks if the binary is Podman
2. **Rootless handling**: Automatically adds `--userns keep-id` for non-root Podman
3. **Buildx compatibility**: Treats Podman as having buildx support

### Code Locations

**Provider Configuration**:
- `providers/docker/provider.yaml` - Docker provider config
- `providers/providers.go` - Built-in provider registry

**Docker Driver**:
- `pkg/driver/docker/docker.go` - Main driver implementation
- `pkg/driver/docker/build.go` - Build operations
- `pkg/docker/helper.go` - Docker/Podman command wrapper

**Key Functions**:
- `NewDockerDriver()` - Creates driver from `workspaceInfo.Agent.Docker.Path`
- `IsPodman()` - Detects Podman via `--version` output
- `ToDockerImageName()` - Normalizes image names (removes non `[a-z0-9\-_]`)

## Issues with Current Approach

### 1. Image Name Mismatch (#1869)
**Problem**: Docker Compose generates image names with underscores, but DevPod looks for hyphens.

**Root Cause**:
```go
// pkg/id/id.go
var dockerImageNameRegEx = regexp.MustCompile(`[^a-z0-9\-_]+`)
func ToDockerImageName(name string) string {
    name = strings.ToLower(name)
    name = dockerImageNameRegEx.ReplaceAllString(name, "")
    return name
}
```
This keeps both `-` and `_`, but Docker Compose may use different conventions than DevPod.

### 2. SELinux Labeling (#1862)
**Problem**: Podman on SELinux systems requires `:Z` or `:z` mount flags.

**Current State**: No automatic SELinux handling in codebase.

**Workarounds**:
- Manual `:Z` in `runArgs`
- Disable SELinux labeling globally
- Set SELinux to permissive

### 3. Rootless User Namespace
**Current Handling**:
```go
// pkg/driver/docker/docker.go:298
if d.Docker.IsPodman() && os.Getuid() != 0 {
    args = append(args, "--userns", "keep-id")
}
```
This works but is Docker-driver specific.

### 4. No Socket/Agent by Default
Podman doesn't use a socket by default, but the Docker provider assumes one exists.

## Provider Configuration Structure

```yaml
name: docker
version: v0.0.1
icon: https://devpod.sh/assets/docker.svg
home: https://github.com/skevetter/devpod
description: DevPod on Docker

optionGroups:
  - options:
      - DOCKER_PATH
      - DOCKER_HOST
      - INACTIVITY_TIMEOUT
      - DOCKER_BUILDER
    name: "Advanced Options"

options:
  DOCKER_PATH:
    description: The path where to find the docker binary.
    default: docker
  DOCKER_HOST:
    global: true
    description: The docker host to use.
  # ... more options

agent:
  containerInactivityTimeout: ${INACTIVITY_TIMEOUT}
  local: true
  docker:
    path: ${DOCKER_PATH}
    builder: ${DOCKER_BUILDER}
    install: false
    env:
      DOCKER_HOST: ${DOCKER_HOST}

exec:
  command: |-
    "${DEVPOD}" helper sh -c "${COMMAND}"
```

## Proposed Podman Provider

### Minimal Implementation

Create `providers/podman/provider.yaml`:

```yaml
name: podman
version: v0.0.1
icon: https://devpod.sh/assets/podman.svg
home: https://github.com/skevetter/devpod
description: |-
  DevPod on Podman - Rootless container engine

optionGroups:
  - options:
      - PODMAN_PATH
      - PODMAN_SOCKET
      - INACTIVITY_TIMEOUT
      - SELINUX_LABEL
    name: "Advanced Options"

options:
  INACTIVITY_TIMEOUT:
    description: "If defined, will automatically stop the container after the inactivity period. Examples: 10m, 1h"
  PODMAN_PATH:
    description: The path where to find the podman binary.
    default: podman
  PODMAN_SOCKET:
    global: true
    description: The podman socket to use (optional).
  SELINUX_LABEL:
    description: "SELinux label for volume mounts. Use 'Z' for private, 'z' for shared, or leave empty."
    default: "Z"
    enum:
      - ""
      - "Z"
      - "z"

agent:
  containerInactivityTimeout: ${INACTIVITY_TIMEOUT}
  local: true
  docker:
    path: ${PODMAN_PATH}
    install: false
    env:
      DOCKER_HOST: ${PODMAN_SOCKET}

exec:
  command: |-
    "${DEVPOD}" helper sh -c "${COMMAND}"
```

### Code Changes Required

#### 1. Add to Built-in Providers
```go
// providers/providers.go
//go:embed podman/provider.yaml
var PodmanProvider string

func GetBuiltInProviders() map[string]string {
    return map[string]string{
        "docker":     DockerProvider,
        "podman":     PodmanProvider,
        "kubernetes": KubernetesProvider,
        "pro":        ProProvider,
    }
}
```

#### 2. SELinux Mount Handling
```go
// pkg/driver/docker/docker.go
func (d *dockerDriver) getSELinuxLabel() string {
    if !d.Docker.IsPodman() {
        return ""
    }
    // Check if SELinux is enabled
    if _, err := os.Stat("/sys/fs/selinux"); err != nil {
        return ""
    }
    // Get from provider config or default to "Z"
    return "Z"
}

// In RunDevContainer:
if label := d.getSELinuxLabel(); label != "" {
    workspaceMount += ":" + label
}
```

#### 3. Image Name Normalization
```go
// pkg/docker/helper.go
func (r *DockerHelper) NormalizeImageName(name string) string {
    if r.IsPodman() {
        // Podman is more flexible with underscores
        return strings.ToLower(name)
    }
    return id.ToDockerImageName(name)
}
```

## Benefits of Dedicated Provider

1. **Clear Intent**: Users explicitly choose Podman
2. **Podman-specific Defaults**: SELinux labels, rootless settings
3. **Better Documentation**: Podman-specific troubleshooting
4. **Future Enhancements**: Podman-only features (pods, systemd integration)
5. **No Breaking Changes**: Docker provider remains unchanged

## Implementation Complexity

**Minimal (Recommended)**:
- Add `providers/podman/provider.yaml` (copy of docker with podman defaults)
- Update `providers/providers.go` to include Podman
- **Effort**: 30 minutes

**Medium**:
- Above + SELinux auto-detection and labeling
- **Effort**: 2-3 hours

**Full**:
- Above + Image name normalization fixes
- Above + Podman-specific optimizations
- **Effort**: 1-2 days

## Recommendation

**Start with Minimal**:
- Create Podman provider as a copy of Docker provider
- Set `PODMAN_PATH=podman` as default
- Add `SELINUX_LABEL` option for users to configure
- Document workarounds in provider description

**Iterate**:
- Gather user feedback
- Add automatic SELinux detection
- Fix image name normalization issues
- Add Podman-specific features (pods, etc.)

This approach:
- ✅ Solves the immediate need (official Podman provider)
- ✅ Minimal code changes
- ✅ No breaking changes to Docker provider
- ✅ Room for future enhancements
- ✅ Clear separation of concerns
