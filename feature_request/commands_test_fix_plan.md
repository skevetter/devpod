# Commands Test Organization and Fix Plan

## Issues Identified

### 1. Path Issues
- Existing tests in `e2e/tests/commands/`: use `initialDir + "/../../bin"`
- New tests in `e2e/tests/commands/agent/`: incorrectly use `initialDir + "/../../../bin"`
- Should be: `initialDir + "/../../../bin"` from agent subdirectory

### 2. Organization Issues
- `agent.go` and `ping.go` are at root of commands/
- New agent tests are in `commands/agent/` subdirectory
- Creates confusion - should all be at same level OR properly organized

### 3. Duplicate Agent Tests
- `commands/agent.go` - tests "agent container" command
- `commands/agent/` directory - tests specific agent container subcommands
- Need to consolidate or clarify separation

## Proposed Organization

### Option A: Flat Structure (Keep Current Pattern)
```
e2e/tests/commands/
├── agent.go              (general agent tests)
├── agent_network_proxy.go
├── agent_port_forward.go
├── agent_ssh_tunnel.go
├── agent_daemon.go
├── agent_credentials.go
├── ping.go
├── framework.go
├── suite.go
└── testdata/
```

### Option B: Organized by Command (Recommended)
```
e2e/tests/commands/
├── agent.go              (general agent container tests)
├── network_proxy.go      (agent container network-proxy)
├── port_forward.go       (agent container port-forward)
├── ssh_tunnel.go         (agent container ssh-tunnel)
├── daemon.go             (agent container daemon)
├── credentials.go        (agent container credentials-server)
├── ping.go               (ping command)
├── framework.go
├── suite.go
└── testdata/
```

## Recommended Approach: Option B

**Rationale**:
1. Keeps all tests at same directory level (no subdirectories)
2. Matches pattern of existing `agent.go` and `ping.go`
3. Clear naming: `network_proxy.go`, `port_forward.go`, etc.
4. Easier path management (all use same `../../bin`)
5. Simpler test discovery

## Implementation Steps

### Step 1: Move Tests from agent/ to commands/
- Move `agent/network_proxy.go` → `commands/network_proxy.go`
- Move `agent/port_forward.go` → `commands/port_forward.go`
- Move `agent/ssh_tunnel.go` → `commands/ssh_tunnel.go`
- Move `agent/daemon.go` → `commands/daemon.go`
- Move `agent/credentials.go` → `commands/credentials.go`

### Step 2: Fix Package Names
- Change `package agent` → `package commands` in all moved files

### Step 3: Fix Paths
- Change `initialDir + "/../../../bin"` → `initialDir + "/../../bin"`
- Change `filepath.Join(initialDir, "../testdata", ...)` → `filepath.Join(initialDir, "testdata", ...)`

### Step 4: Update Test Descriptions
- Keep `DevPodDescribe` but make descriptions more specific
- Example: `"agent container network-proxy"` instead of just `"network-proxy command"`

### Step 5: Remove agent/ Directory
- Delete `e2e/tests/commands/agent/` directory
- Remove from e2e suite registration

### Step 6: Update Suite Registration
- Remove `_ "github.com/skevetter/devpod/e2e/tests/commands/agent"`
- Tests will be picked up from commands package directly

## Expected Final Structure

```
e2e/tests/commands/
├── agent.go              # General agent container command test
├── credentials.go        # agent container credentials-server tests
├── daemon.go             # agent container daemon tests
├── framework.go          # DevPodDescribe helper
├── network_proxy.go      # agent container network-proxy tests
├── ping.go               # ping/binary tests
├── port_forward.go       # agent container port-forward tests
├── ssh_tunnel.go         # agent container ssh-tunnel tests
├── suite.go              # Test suite runner
└── testdata/
    └── simple-app/
```

## Test Labels

All tests will use consistent labels:
- `agent` - General agent tests
- `network-proxy` - Network proxy specific
- `port-forward` - Port forward specific
- `ssh-tunnel` - SSH tunnel specific
- `daemon` - Daemon specific
- `credentials` - Credentials specific
- `ping` - Ping/binary tests

## Benefits

1. ✅ Consistent directory structure
2. ✅ All tests at same level
3. ✅ Correct path references
4. ✅ Simpler test discovery
5. ✅ Easier to maintain
6. ✅ Follows existing pattern
7. ✅ Tests will actually run

## Success Criteria

- ✅ All tests compile
- ✅ All tests discovered by ginkgo
- ✅ Tests run without path errors
- ✅ Consistent with existing commands tests
- ✅ Clear file organization
