# Network Test Infrastructure Improvement Plan

## Current Issues

### 1. Naming Inconsistencies
- `network.go` - too generic, tests transport layer
- `proxy.go` - tests connection tracker, not proxy
- `grpc_proxy.go` - tests gRPC proxy
- `server_integration.go` - unclear what server
- `network_traffic.go` - overlaps with other tests

### 2. Poor Organization
- Unit-style tests (transport, proxy) mixed with integration tests (port_forward, credentials)
- No clear separation between:
  - Transport layer tests
  - Proxy/server tests
  - Integration tests (workspace-based)
  - Connection management tests

### 3. File Grouping Issues
- Connection-related: `connection_lifecycle.go`, `connection_tracking.go`
- Proxy-related: `proxy.go`, `grpc_proxy.go`, `server_running.go`
- Integration: `port_forward.go`, `ssh_tunnel_traffic.go`, `credentials.go`
- Platform-specific: `kubernetes.go`, `container_compatibility.go`, `daemon_integration.go`

## Proposed Structure

### Organize by Feature Area

```
e2e/tests/network/
├── transport/           # Transport layer tests
│   └── transport.go     (renamed from network.go)
├── proxy/              # Proxy and server tests
│   ├── connection_tracker.go  (renamed from proxy.go)
│   ├── grpc.go         (renamed from grpc_proxy.go)
│   └── server.go       (renamed from server_running.go)
├── connection/         # Connection management
│   ├── lifecycle.go    (renamed from connection_lifecycle.go)
│   └── tracking.go     (renamed from connection_tracking.go)
├── integration/        # Workspace integration tests
│   ├── port_forward.go
│   ├── ssh_tunnel.go   (renamed from ssh_tunnel_traffic.go)
│   ├── credentials.go
│   └── traffic.go      (renamed from network_traffic.go)
├── platform/           # Platform-specific tests
│   ├── kubernetes.go
│   ├── container.go    (renamed from container_compatibility.go)
│   └── daemon.go       (renamed from daemon_integration.go)
├── heartbeat.go        # Standalone heartbeat tests
├── framework.go
├── helpers.go
├── suite.go
└── testdata/
```

## Naming Conventions

### File Naming
- Use descriptive names that indicate what is being tested
- Avoid generic names like `network.go`, `proxy.go`
- Use singular form: `connection.go` not `connections.go`
- Group related tests in subdirectories

### Test Description Naming
- Use clear, action-oriented descriptions
- Format: `DevPodDescribe("feature area", ...)`
- Examples:
  - "transport layer" not "network transport test suite"
  - "connection tracker" not "testing connection tracker"
  - "port forwarding" not "port forwarding"

## Implementation Steps

### Step 1: Create Subdirectories
```bash
mkdir -p transport proxy connection integration platform
```

### Step 2: Move and Rename Transport Tests
- `network.go` → `transport/transport.go`

### Step 3: Move and Rename Proxy Tests
- `proxy.go` → `proxy/connection_tracker.go`
- `grpc_proxy.go` → `proxy/grpc.go`
- `server_running.go` → `proxy/server.go`
- `server_integration.go` → `proxy/integration.go`

### Step 4: Move Connection Tests
- `connection_lifecycle.go` → `connection/lifecycle.go`
- `connection_tracking.go` → `connection/tracking.go`

### Step 5: Move Integration Tests
- `port_forward.go` → `integration/port_forward.go`
- `ssh_tunnel_traffic.go` → `integration/ssh_tunnel.go`
- `credentials.go` → `integration/credentials.go`
- `network_traffic.go` → `integration/traffic.go`

### Step 6: Move Platform Tests
- `kubernetes.go` → `platform/kubernetes.go`
- `container_compatibility.go` → `platform/container.go`
- `daemon_integration.go` → `platform/daemon.go`

### Step 7: Keep at Root Level
- `heartbeat_timeout.go` → `heartbeat.go` (rename)
- `framework.go`
- `helpers.go`
- `suite.go`
- `testdata/`

### Step 8: Update Test Descriptions
Clean up verbose test descriptions to be more concise.

## Expected Benefits

1. **Clear Organization** - Tests grouped by feature area
2. **Better Navigation** - Easy to find specific test types
3. **Scalability** - Easy to add new tests in appropriate subdirectories
4. **Reduced Confusion** - Descriptive file names
5. **Maintainability** - Related tests are co-located
