# Network Proxy Test Migration Plan

## Objective
Migrate all tests from `e2e/tests/networkproxy` to `e2e/tests/network` and refactor duplicate code.

## Current State Analysis

### networkproxy directory (15 files):
- connection_lifecycle.go
- connection_tracking.go
- container_compatibility.go
- credentials.go
- daemon_integration.go
- heartbeat_timeout.go
- kubernetes.go
- network_traffic.go
- port_forward.go (basic network test)
- port_forward2.go (HTTP server test) - **DUPLICATE LOGIC**
- server_running.go
- ssh_tunnel_traffic.go
- framework.go
- helper.go (utility functions)
- suite.go

### network directory (5 files):
- network.go (transport tests)
- grpc_proxy.go
- proxy.go
- server_integration.go
- suite.go

## Identified Issues

### 1. Duplicate Code
- **port_forward.go** and **port_forward2.go** have similar workspace setup logic
- Both files create workspace, setup docker provider, run DevPodUp
- Should be consolidated into single file with multiple test cases

### 2. Shared Utilities
- **helper.go** contains `verifyPortListening()` and `verifyNetworkProxyRunning()`
- These should be moved to a shared helper file in network directory

### 3. Framework Duplication
- Both directories have their own `framework.go` with DevPodDescribe
- Should use single framework in network directory

## Migration Steps

### Step 1: Consolidate Port Forward Tests
- Merge port_forward.go and port_forward2.go into single port_forward.go
- Create separate test cases for basic and HTTP scenarios
- Extract common setup into BeforeEach

### Step 2: Create Shared Helpers
- Move helper.go utilities to network/helpers.go
- Add any additional helper functions needed

### Step 3: Migrate Test Files
Move files from networkproxy to network:
- connection_lifecycle.go → network/connection_lifecycle.go
- connection_tracking.go → network/connection_tracking.go
- container_compatibility.go → network/container_compatibility.go
- credentials.go → network/credentials.go
- daemon_integration.go → network/daemon_integration.go
- heartbeat_timeout.go → network/heartbeat_timeout.go
- kubernetes.go → network/kubernetes.go
- network_traffic.go → network/network_traffic.go
- port_forward.go (consolidated) → network/port_forward.go
- server_running.go → network/server_running.go
- ssh_tunnel_traffic.go → network/ssh_tunnel_traffic.go

### Step 4: Update Package Names
- Change all `package networkproxy` to `package network`
- Update DevPodDescribe label from `[networkproxy]` to `[network]`

### Step 5: Move Test Data
- Move testdata directory from networkproxy to network

### Step 6: Update Suite
- Update suite.go to reflect "Network E2E Suite"

### Step 7: Cleanup
- Remove e2e/tests/networkproxy directory
- Verify all tests compile and run

## Refactoring Opportunities

### Common Test Setup Pattern
Extract repeated code:
```go
// Before (repeated in many files)
f := framework.NewDefaultFramework(initialDir + "/../../bin")
_ = f.DevPodProviderDelete(ctx, "docker")
err := f.DevPodProviderAdd(ctx, "docker")
err = f.DevPodProviderUse(ctx, "docker")

// After (helper function)
func setupDockerProvider(ctx context.Context, f *framework.Framework) error
```

### Port Forward Consolidation
```go
// Single file with multiple contexts:
var _ = DevPodDescribe("port forwarding", func() {
    ginkgo.Context("basic network operations", ...)
    ginkgo.Context("HTTP server forwarding", ...)
})
```

## Expected Outcome
- Single network test directory with all network-related tests
- Reduced code duplication
- Clearer test organization
- Easier maintenance
