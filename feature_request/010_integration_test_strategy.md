# Integration Test Strategy - Network Proxy

**Date:** 2025-12-05
**Purpose:** Validate network proxy functionality in real devcontainer environments

---

## Overview

This document outlines the integration test strategy for the new network proxy features, following the existing e2e/tests/up pattern of spinning up actual devcontainers and verifying end-to-end functionality.

---

## Current e2e/tests/up Pattern Analysis

### Existing Test Structure
```
e2e/tests/up/
├── up.go                    - Main test suite
├── helper.go                - Test utilities
├── framework.go             - Test annotations
├── docker.go                - Docker-specific tests
├── docker_compose.go        - Compose tests
└── testdata/                - Test fixtures
```

### Key Patterns Observed

1. **Test Lifecycle:**
   ```go
   - Setup provider (docker/kubernetes)
   - DevPodUp() - Spin up workspace
   - DevPodSSH() - Execute commands in container
   - Verify functionality
   - DeferCleanup - Cleanup workspace
   ```

2. **Test Utilities:**
   - `setupWorkspace()` - Copy testdata to temp dir
   - `setupDockerProvider()` - Configure provider
   - `devPodUpAndFindWorkspace()` - Create and find workspace
   - `findMessage()` - Verify log output

3. **Verification Methods:**
   - SSH into container and run commands
   - Check environment variables
   - Verify files exist
   - Parse log output
   - Inspect container state

---

## New Network Proxy Features to Test

### 1. Network Proxy Server
- Server starts with daemon
- Listens on configured port
- Handles gRPC/HTTP traffic
- Connection tracking works
- Heartbeat removes stale connections

### 2. Port Forwarding
- Forward local port to remote service
- Bidirectional data transfer
- Multiple simultaneous forwards
- Cleanup on shutdown

### 3. SSH Tunneling
- Create tunnel to remote service
- Data transfer through tunnel
- Random port allocation
- Cleanup on shutdown

### 4. Daemon Integration
- Network proxy starts with daemon
- Configuration from daemon config
- Graceful shutdown
- Error handling

### 5. CLI Commands
- `network-proxy` command works
- `port-forward` command works
- `ssh-tunnel` command works
- Signal handling (Ctrl+C)

---

## Integration Test Plan

### Test Suite Structure

```
e2e/tests/network-proxy/
├── network_proxy_test.go        - Main test suite
├── port_forward_test.go         - Port forwarding tests
├── ssh_tunnel_test.go           - SSH tunnel tests
├── daemon_integration_test.go   - Daemon integration tests
├── helper.go                    - Test utilities
├── framework.go                 - Test annotations
└── testdata/
    ├── simple-app/              - Simple HTTP server
    │   ├── devcontainer.json
    │   └── server.py
    ├── with-network-proxy/      - With network proxy config
    │   ├── devcontainer.json
    │   └── daemon_config.json
    └── multi-service/           - Multiple services
        ├── devcontainer.json
        └── docker-compose.yml
```

---

## Test Cases

### Suite 1: Network Proxy Server Integration

#### Test 1.1: Network Proxy Starts with Daemon
```go
It("starts network proxy with daemon", func() {
    // Setup workspace with network proxy config
    tempDir := setupWorkspaceWithNetworkProxy()

    // Start workspace
    err := f.DevPodUp(ctx, tempDir)
    ExpectNoError(err)

    // Verify network proxy is running
    out, err := f.DevPodSSH(ctx, tempDir, "netstat -tuln | grep 9090")
    ExpectNoError(err)
    Expect(out).To(ContainSubstring("LISTEN"))

    // Verify process is running
    out, err = f.DevPodSSH(ctx, tempDir, "ps aux | grep network-proxy")
    ExpectNoError(err)
    Expect(out).To(ContainSubstring("network-proxy"))
})
```

#### Test 1.2: Network Proxy Configuration
```go
It("respects network proxy configuration", func() {
    // Setup with custom config
    config := NetworkProxyConfig{
        Enabled: true,
        Addr: "localhost:9999",
    }
    tempDir := setupWorkspaceWithConfig(config)

    // Start workspace
    err := f.DevPodUp(ctx, tempDir)
    ExpectNoError(err)

    // Verify listening on custom port
    out, err := f.DevPodSSH(ctx, tempDir, "netstat -tuln | grep 9999")
    ExpectNoError(err)
    Expect(out).To(ContainSubstring("LISTEN"))
})
```

#### Test 1.3: Network Proxy Disabled by Default
```go
It("does not start network proxy when disabled", func() {
    // Setup without network proxy config
    tempDir := setupWorkspace("testdata/simple-app")

    // Start workspace
    err := f.DevPodUp(ctx, tempDir)
    ExpectNoError(err)

    // Verify network proxy is NOT running
    out, err := f.DevPodSSH(ctx, tempDir, "ps aux | grep network-proxy")
    ExpectNoError(err)
    Expect(out).NotTo(ContainSubstring("network-proxy"))
})
```

### Suite 2: Port Forwarding Integration

#### Test 2.1: Basic Port Forward
```go
It("forwards port to service in container", func() {
    // Setup workspace with HTTP server
    tempDir := setupWorkspace("testdata/simple-app")
    err := f.DevPodUp(ctx, tempDir)
    ExpectNoError(err)

    // Start HTTP server in container
    go f.DevPodSSH(ctx, tempDir, "python3 -m http.server 8000")
    time.Sleep(2 * time.Second)

    // Start port forward in background
    portForwardCmd := f.DevPodExec(ctx, "agent", "container", "port-forward",
        "--local-port", "8080",
        "--remote-addr", "localhost:8000")
    go portForwardCmd.Run()
    time.Sleep(1 * time.Second)

    // Verify port forward works
    resp, err := http.Get("http://localhost:8080")
    ExpectNoError(err)
    Expect(resp.StatusCode).To(Equal(200))

    // Cleanup
    portForwardCmd.Process.Signal(syscall.SIGTERM)
})
```

#### Test 2.2: Multiple Port Forwards
```go
It("handles multiple simultaneous port forwards", func() {
    // Setup workspace with multiple services
    tempDir := setupWorkspace("testdata/multi-service")
    err := f.DevPodUp(ctx, tempDir)
    ExpectNoError(err)

    // Start multiple services
    go f.DevPodSSH(ctx, tempDir, "python3 -m http.server 8000")
    go f.DevPodSSH(ctx, tempDir, "python3 -m http.server 8001")
    time.Sleep(2 * time.Second)

    // Start multiple port forwards
    pf1 := startPortForward(f, ctx, "8080", "localhost:8000")
    pf2 := startPortForward(f, ctx, "8081", "localhost:8001")
    defer pf1.Stop()
    defer pf2.Stop()

    // Verify both work
    resp1, err := http.Get("http://localhost:8080")
    ExpectNoError(err)
    Expect(resp1.StatusCode).To(Equal(200))

    resp2, err := http.Get("http://localhost:8081")
    ExpectNoError(err)
    Expect(resp2.StatusCode).To(Equal(200))
})
```

#### Test 2.3: Port Forward Cleanup
```go
It("cleans up port forward on shutdown", func() {
    tempDir := setupWorkspace("testdata/simple-app")
    err := f.DevPodUp(ctx, tempDir)
    ExpectNoError(err)

    // Start service
    go f.DevPodSSH(ctx, tempDir, "python3 -m http.server 8000")
    time.Sleep(2 * time.Second)

    // Start and stop port forward
    pf := startPortForward(f, ctx, "8080", "localhost:8000")
    time.Sleep(1 * time.Second)
    pf.Stop()

    // Verify port is released
    _, err = net.Listen("tcp", "localhost:8080")
    ExpectNoError(err) // Should succeed if port is free
})
```

### Suite 3: SSH Tunnel Integration

#### Test 3.1: Basic SSH Tunnel
```go
It("creates SSH tunnel to service", func() {
    tempDir := setupWorkspace("testdata/simple-app")
    err := f.DevPodUp(ctx, tempDir)
    ExpectNoError(err)

    // Start SSH server in container
    f.DevPodSSH(ctx, tempDir, "sudo service ssh start")

    // Create SSH tunnel
    tunnel := startSSHTunnel(f, ctx, "localhost:0", "localhost:22")
    defer tunnel.Stop()

    // Get assigned port
    localAddr := tunnel.LocalAddr()

    // Verify tunnel works
    conn, err := net.Dial("tcp", localAddr)
    ExpectNoError(err)
    conn.Close()
})
```

#### Test 3.2: SSH Tunnel Data Transfer
```go
It("transfers data through SSH tunnel", func() {
    tempDir := setupWorkspace("testdata/simple-app")
    err := f.DevPodUp(ctx, tempDir)
    ExpectNoError(err)

    // Start echo server
    go f.DevPodSSH(ctx, tempDir, "nc -l -p 9999")

    // Create tunnel
    tunnel := startSSHTunnel(f, ctx, "localhost:0", "localhost:9999")
    defer tunnel.Stop()

    // Send data through tunnel
    conn, err := net.Dial("tcp", tunnel.LocalAddr())
    ExpectNoError(err)
    defer conn.Close()

    testData := "hello world"
    conn.Write([]byte(testData))

    // Verify data received
    buf := make([]byte, 1024)
    n, err := conn.Read(buf)
    ExpectNoError(err)
    Expect(string(buf[:n])).To(Equal(testData))
})
```

### Suite 4: Connection Tracking Integration

#### Test 4.1: Connection Tracking
```go
It("tracks active connections", func() {
    tempDir := setupWorkspaceWithNetworkProxy()
    err := f.DevPodUp(ctx, tempDir)
    ExpectNoError(err)

    // Create connections
    pf1 := startPortForward(f, ctx, "8080", "localhost:8000")
    pf2 := startPortForward(f, ctx, "8081", "localhost:8001")
    defer pf1.Stop()
    defer pf2.Stop()

    // Verify connections are tracked
    out, err := f.DevPodSSH(ctx, tempDir,
        "curl http://localhost:9090/connections")
    ExpectNoError(err)

    var conns []Connection
    json.Unmarshal([]byte(out), &conns)
    Expect(len(conns)).To(BeNumerically(">=", 2))
})
```

#### Test 4.2: Heartbeat Removes Stale Connections
```go
It("removes stale connections via heartbeat", func() {
    config := NetworkProxyConfig{
        Enabled: true,
        Heartbeat: HeartbeatConfig{
            Interval: 1 * time.Second,
            Timeout: 2 * time.Second,
        },
    }
    tempDir := setupWorkspaceWithConfig(config)
    err := f.DevPodUp(ctx, tempDir)
    ExpectNoError(err)

    // Create connection
    pf := startPortForward(f, ctx, "8080", "localhost:8000")
    time.Sleep(1 * time.Second)

    // Kill connection without cleanup
    pf.Process.Kill()

    // Wait for heartbeat to detect
    time.Sleep(3 * time.Second)

    // Verify connection removed
    out, err := f.DevPodSSH(ctx, tempDir,
        "curl http://localhost:9090/connections")
    ExpectNoError(err)

    var conns []Connection
    json.Unmarshal([]byte(out), &conns)
    Expect(len(conns)).To(Equal(0))
})
```

### Suite 5: End-to-End Scenarios

#### Test 5.1: Full Workflow
```go
It("completes full network proxy workflow", func() {
    // 1. Setup workspace with network proxy
    tempDir := setupWorkspaceWithNetworkProxy()
    err := f.DevPodUp(ctx, tempDir)
    ExpectNoError(err)

    // 2. Verify network proxy running
    out, err := f.DevPodSSH(ctx, tempDir, "ps aux | grep network-proxy")
    ExpectNoError(err)
    Expect(out).To(ContainSubstring("network-proxy"))

    // 3. Start service in container
    go f.DevPodSSH(ctx, tempDir, "python3 -m http.server 8000")
    time.Sleep(2 * time.Second)

    // 4. Forward port
    pf := startPortForward(f, ctx, "8080", "localhost:8000")
    defer pf.Stop()

    // 5. Access service through forward
    resp, err := http.Get("http://localhost:8080")
    ExpectNoError(err)
    Expect(resp.StatusCode).To(Equal(200))

    // 6. Create SSH tunnel
    tunnel := startSSHTunnel(f, ctx, "localhost:0", "localhost:22")
    defer tunnel.Stop()

    // 7. Verify tunnel works
    conn, err := net.Dial("tcp", tunnel.LocalAddr())
    ExpectNoError(err)
    conn.Close()

    // 8. Cleanup and verify
    pf.Stop()
    tunnel.Stop()

    // 9. Verify resources released
    _, err = net.Listen("tcp", "localhost:8080")
    ExpectNoError(err)
})
```

---

## Test Utilities to Implement

### Helper Functions

```go
// setupWorkspaceWithNetworkProxy creates workspace with network proxy enabled
func setupWorkspaceWithNetworkProxy() string {
    config := NetworkProxyConfig{
        Enabled: true,
        Addr: "localhost:9090",
    }
    return setupWorkspaceWithConfig(config)
}

// setupWorkspaceWithConfig creates workspace with custom config
func setupWorkspaceWithConfig(config NetworkProxyConfig) string {
    tempDir, _ := framework.CopyToTempDir("testdata/with-network-proxy")

    // Write daemon config
    configJSON, _ := json.Marshal(map[string]interface{}{
        "networkProxy": config,
    })
    os.WriteFile(filepath.Join(tempDir, "daemon_config.json"), configJSON, 0644)

    return tempDir
}

// startPortForward starts port forward in background
func startPortForward(f *framework.Framework, ctx context.Context, localPort, remoteAddr string) *PortForward {
    cmd := f.DevPodExec(ctx, "agent", "container", "port-forward",
        "--local-port", localPort,
        "--remote-addr", remoteAddr)

    go cmd.Run()
    time.Sleep(500 * time.Millisecond)

    return &PortForward{cmd: cmd, localPort: localPort}
}

// startSSHTunnel starts SSH tunnel in background
func startSSHTunnel(f *framework.Framework, ctx context.Context, localAddr, remoteAddr string) *SSHTunnel {
    cmd := f.DevPodExec(ctx, "agent", "container", "ssh-tunnel",
        "--local-addr", localAddr,
        "--remote-addr", remoteAddr)

    stdout, _ := cmd.StdoutPipe()
    go cmd.Run()

    // Parse local address from output
    scanner := bufio.NewScanner(stdout)
    var actualAddr string
    for scanner.Scan() {
        line := scanner.Text()
        if strings.Contains(line, "SSH tunnel:") {
            // Extract address
            parts := strings.Split(line, " ")
            actualAddr = parts[2]
            break
        }
    }

    return &SSHTunnel{cmd: cmd, localAddr: actualAddr}
}

// verifyNetworkProxyRunning checks if network proxy is running
func verifyNetworkProxyRunning(f *framework.Framework, ctx context.Context, workspace string) error {
    out, err := f.DevPodSSH(ctx, workspace, "ps aux | grep network-proxy")
    if err != nil {
        return err
    }
    if !strings.Contains(out, "network-proxy") {
        return fmt.Errorf("network proxy not running")
    }
    return nil
}

// verifyPortListening checks if port is listening
func verifyPortListening(f *framework.Framework, ctx context.Context, workspace string, port int) error {
    out, err := f.DevPodSSH(ctx, workspace, fmt.Sprintf("netstat -tuln | grep %d", port))
    if err != nil {
        return err
    }
    if !strings.Contains(out, "LISTEN") {
        return fmt.Errorf("port %d not listening", port)
    }
    return nil
}
```

---

## Test Data Structure

### testdata/simple-app/
```
simple-app/
├── devcontainer.json
├── server.py           # Simple HTTP server
└── README.md
```

**devcontainer.json:**
```json
{
  "name": "Simple App",
  "image": "python:3.11",
  "postCreateCommand": "pip install flask"
}
```

**server.py:**
```python
from flask import Flask
app = Flask(__name__)

@app.route('/')
def hello():
    return 'Hello from DevPod!'

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=8000)
```

### testdata/with-network-proxy/
```
with-network-proxy/
├── devcontainer.json
├── daemon_config.json
└── server.py
```

**daemon_config.json:**
```json
{
  "networkProxy": {
    "enabled": true,
    "addr": "localhost:9090",
    "grpcTarget": "",
    "httpTarget": ""
  }
}
```

### testdata/multi-service/
```
multi-service/
├── devcontainer.json
├── docker-compose.yml
├── api/
│   └── server.py
└── db/
    └── init.sql
```

---

## Execution Strategy

### Phase 1: Basic Tests (Week 1)
- Test 1.1, 1.2, 1.3 - Network proxy server
- Test 2.1 - Basic port forward
- Test 3.1 - Basic SSH tunnel

### Phase 2: Advanced Tests (Week 2)
- Test 2.2, 2.3 - Multiple forwards, cleanup
- Test 3.2 - SSH tunnel data transfer
- Test 4.1, 4.2 - Connection tracking, heartbeat

### Phase 3: E2E Tests (Week 3)
- Test 5.1 - Full workflow
- Performance tests
- Stress tests

---

## Success Criteria

- ✅ All tests pass consistently
- ✅ Tests run in CI/CD pipeline
- ✅ Coverage of all major features
- ✅ Tests complete in < 10 minutes
- ✅ Clear failure messages
- ✅ Proper cleanup (no leaked resources)

---

## CI/CD Integration

### GitHub Actions Workflow
```yaml
name: Network Proxy E2E Tests

on: [push, pull_request]

jobs:
  e2e-network-proxy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      - name: Build DevPod
        run: ./hack/build-e2e.sh
      - name: Run Network Proxy E2E Tests
        run: |
          cd e2e/tests/network-proxy
          ginkgo -v --label-filter="network-proxy"
```

---

## Next Steps

1. Create `e2e/tests/network-proxy/` directory structure
2. Implement helper functions
3. Create test data fixtures
4. Implement Phase 1 tests
5. Run and validate tests
6. Iterate based on results
7. Add to CI/CD pipeline

---

**Status:** STRATEGY COMPLETE - READY FOR IMPLEMENTATION
