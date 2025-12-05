# Agent Container Commands Integration Testing Plan

## Overview
Add integration tests for new network-related commands in `cmd/agent/container/`:
- `network-proxy` - Network proxy server
- `port-forward` - Port forwarding
- `ssh-tunnel` - SSH tunneling
- Enhanced `daemon` - Network daemon with monitoring
- Enhanced `credentials-server` - Credentials with HTTP tunnel

## New Commands to Test

### 1. network-proxy
**File**: `cmd/agent/container/network_proxy.go`
**Flags**:
- `--addr` (default: localhost:9090)
- `--grpc-target` (optional)
- `--http-target` (optional)

**Test Cases**:
- Start network proxy server
- Verify server is listening on specified address
- Test with gRPC target configuration
- Test with HTTP target configuration

### 2. port-forward
**File**: `cmd/agent/container/port_forward.go`
**Flags**:
- `--local-port` (required)
- `--remote-addr` (required)

**Test Cases**:
- Forward local port to remote address
- Verify port forwarding is active
- Test data transfer through forwarded port
- Test graceful shutdown

### 3. ssh-tunnel
**File**: `cmd/agent/container/ssh_tunnel.go`
**Flags**:
- `--local-addr` (default: localhost:0)
- `--remote-addr` (required)

**Test Cases**:
- Create SSH tunnel
- Verify tunnel is established
- Test with random port allocation
- Test connection through tunnel

### 4. daemon (enhanced)
**File**: `cmd/agent/container/daemon.go`
**New Features**:
- Network daemon integration
- Container activity monitoring
- Timeout management

**Test Cases**:
- Start daemon with network support
- Verify daemon is running
- Test timeout functionality
- Test graceful shutdown

### 5. credentials-server (enhanced)
**File**: `cmd/agent/container/credentials_server.go`
**New Features**:
- HTTP tunnel support
- Network-aware credential forwarding

**Test Cases**:
- Start credentials server
- Verify credential forwarding works
- Test with HTTP tunnel client

## Test Organization

### Location
`e2e/tests/commands/agent/` - Create subdirectory for agent container tests

### Structure
```
e2e/tests/commands/agent/
├── suite.go
├── network_proxy.go
├── port_forward.go
├── ssh_tunnel.go
├── daemon.go
└── credentials.go
```

## Test Implementation Strategy

### Common Setup
All tests will:
1. Create a workspace with docker provider
2. Run `devpod up` to start container
3. Execute agent container commands via SSH
4. Verify command behavior
5. Clean up workspace

### Test Pattern
```go
var _ = DevPodDescribe("agent container commands", func() {
    ginkgo.Context("network-proxy", func() {
        ginkgo.It("starts network proxy server", func() {
            // Setup workspace
            // Run: devpod agent container network-proxy --addr localhost:9090
            // Verify server is running
            // Cleanup
        })
    })
})
```

### Verification Methods
- Check process is running: `ps aux | grep <command>`
- Check port is listening: `netstat -tuln | grep <port>`
- Test connectivity: `curl`, `nc`, or similar tools
- Check logs/output for expected messages

## Implementation Steps

### Step 1: Create Agent Test Directory
```bash
mkdir -p e2e/tests/commands/agent
```

### Step 2: Create Suite File
Create `suite.go` with test runner and DevPodDescribe helper

### Step 3: Implement Network Proxy Tests
File: `network_proxy.go`
- Test basic server start
- Test with gRPC target
- Test with HTTP target
- Verify server responds

### Step 4: Implement Port Forward Tests
File: `port_forward.go`
- Test basic port forwarding
- Test data transfer
- Test error handling

### Step 5: Implement SSH Tunnel Tests
File: `ssh_tunnel.go`
- Test tunnel creation
- Test random port allocation
- Test connection through tunnel

### Step 6: Implement Daemon Tests
File: `daemon.go`
- Test daemon startup
- Test network integration
- Test timeout behavior

### Step 7: Implement Credentials Tests
File: `credentials.go`
- Test credentials server
- Test HTTP tunnel integration

### Step 8: Register Tests
Add to `e2e/e2e_suite_test.go`:
```go
_ "github.com/skevetter/devpod/e2e/tests/commands/agent"
```

## Test Labels

Use hierarchical labels for filtering:
- `[commands:agent]` - All agent container tests
- `[commands:agent:network-proxy]` - Network proxy tests
- `[commands:agent:port-forward]` - Port forward tests
- `[commands:agent:ssh-tunnel]` - SSH tunnel tests
- `[commands:agent:daemon]` - Daemon tests
- `[commands:agent:credentials]` - Credentials tests

## Expected Outcomes

1. Comprehensive test coverage for new network commands
2. Validation that commands work in real workspace environment
3. Early detection of integration issues
4. Documentation through tests of command usage
5. Confidence in command functionality

## Success Criteria

- ✅ All agent container commands have integration tests
- ✅ Tests run successfully in CI/CD
- ✅ Tests can be run independently via labels
- ✅ Tests validate actual command behavior, not just execution
- ✅ Tests are maintainable and well-documented
