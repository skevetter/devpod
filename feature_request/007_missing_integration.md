# Missing Integration

## What We Built vs What's Actually Used

### Built ✅
- Transport interface
- HTTP transport
- Stdio transport
- Fallback transport
- Connection pooling
- CLI flags (`--http-tunnel-client`, `--http-tunnel-port`)
- `setupTransport()` helper function

### Actually Used ❌
**NONE OF IT**

The credentials server still uses the original stdio-only implementation.

---

## The Problem

### Current Flow (What Actually Runs)
```go
// cmd/agent/container/credentials_server.go
func (cmd *CredentialsServerCmd) Run(ctx context.Context, port int) error {
    // Creates stdio tunnel client (ALWAYS)
    tunnelClient, err := tunnelserver.NewTunnelClient(os.Stdin, os.Stdout, true, ExitCodeIO)

    // ... setup ...

    // Runs credentials server with stdio client
    return credentials.RunCredentialsServer(ctx, port, tunnelClient, log)
}
```

### What We Built (Never Called)
```go
// cmd/agent/container/credentials_server.go
func (cmd *CredentialsServerCmd) setupTransport(log log.Logger) network.Transport {
    // This function is NEVER CALLED
    if cmd.HTTPTunnelClient != "" {
        httpTransport := network.NewHTTPTransport(host, port)
        return network.NewFallbackTransport(httpTransport, stdioTransport)
    }
    return stdioTransport
}
```

---

## What's Missing

### 1. Wire Transport into Credentials Server
The credentials server needs to use our transport layer instead of direct stdio.

**Problem:** `credentials.RunCredentialsServer()` expects a `tunnel.TunnelClient`, not our `network.Transport`.

**Solution Options:**

#### Option A: Adapter Pattern
Create adapter to convert `network.Transport` → `tunnel.TunnelClient`

```go
type TransportTunnelClient struct {
    transport network.Transport
}

func (t *TransportTunnelClient) Ping(ctx context.Context, req *tunnel.Empty) (*tunnel.Empty, error) {
    conn, err := t.transport.Dial(ctx, "")
    // ... implement tunnel.TunnelClient interface
}
```

#### Option B: Modify Credentials Server
Change `credentials.RunCredentialsServer()` to accept `network.Transport`

```go
func RunCredentialsServer(
    ctx context.Context,
    port int,
    transport network.Transport,  // Changed from tunnel.TunnelClient
    log log.Logger,
) error {
    // Use transport instead of client
}
```

#### Option C: Keep Separate (Current State)
Don't integrate - keep as separate unused code.

---

### 2. HTTP Tunnel Server on Client Side
We have HTTP client transport, but **no server** to connect to.

**Missing:** Server that listens on `localhost:8080` and forwards to credentials.

```go
// This doesn't exist anywhere
func StartHTTPTunnelServer(port int) {
    http.HandleFunc("/tunnel", func(w http.ResponseWriter, r *http.Request) {
        // Forward to actual credentials server
    })
    http.ListenAndServe(":"+port, nil)
}
```

---

### 3. Actual Network Protocol
Our transports can dial, but **what do they send/receive**?

**Missing:** Protocol definition for credentials over HTTP.

```go
// What format?
// JSON? gRPC? Raw bytes?
// How to encode git credentials request?
// How to decode response?
```

---

## Why PR #1836 Works and Ours Doesn't

### PR #1836 Has:
1. ✅ **Full gRPC proxy** - Proxies gRPC calls over network
2. ✅ **HTTP proxy handler** - Handles HTTP requests
3. ✅ **Tailscale integration** - Peer-to-peer networking
4. ✅ **Server side** - Runs on client machine
5. ✅ **Client side** - Runs in workspace
6. ✅ **Protocol** - gRPC/HTTP protocol defined
7. ✅ **Integration** - Fully wired together

### We Have:
1. ✅ Transport abstraction (can dial)
2. ❌ No server side
3. ❌ No protocol
4. ❌ No integration
5. ❌ No actual communication

---

## Minimal Integration Needed

To make our code actually work:

### Step 1: Create HTTP Tunnel Server (Client Side)
```go
// pkg/daemon/local/http_tunnel_server.go
func StartHTTPTunnelServer(ctx context.Context, port int, tunnelClient tunnel.TunnelClient) error {
    http.HandleFunc("/tunnel", func(w http.ResponseWriter, r *http.Request) {
        // Read request from workspace
        body, _ := io.ReadAll(r.Body)

        // Forward to local credentials server via tunnelClient
        response, err := tunnelClient.ForwardRequest(ctx, body)

        // Send response back
        w.Write(response)
    })

    return http.ListenAndServe(":"+strconv.Itoa(port), nil)
}
```

### Step 2: Use Transport in Credentials Server
```go
// cmd/agent/container/credentials_server.go
func (cmd *CredentialsServerCmd) Run(ctx context.Context, port int) error {
    // Use our transport
    transport := cmd.setupTransport(log)

    // Create tunnel client from transport
    tunnelClient := NewTransportTunnelClient(transport)

    // Run credentials server
    return credentials.RunCredentialsServer(ctx, port, tunnelClient, log)
}
```

### Step 3: Implement Transport Protocol
```go
// pkg/daemon/workspace/network/protocol.go
type CredentialsRequest struct {
    Type string // "git", "docker", "ssh"
    Data []byte
}

type CredentialsResponse struct {
    Data  []byte
    Error string
}

func (t *HTTPTransport) SendRequest(ctx context.Context, req *CredentialsRequest) (*CredentialsResponse, error) {
    conn, err := t.Dial(ctx, t.target)
    // Encode request as JSON
    // Send over connection
    // Decode response
    return response, nil
}
```

---

## Estimated Effort

### Minimal Integration (Make It Work)
**Time:** 2-3 days

1. Create HTTP tunnel server (client side) - 4 hours
2. Create transport adapter for tunnel.TunnelClient - 4 hours
3. Define simple JSON protocol - 2 hours
4. Wire everything together - 4 hours
5. Test end-to-end - 4 hours

### Full PR #1836 Alignment
**Time:** 2-3 weeks

1. Add Tailscale integration - 1 week
2. Implement gRPC proxy - 1 week
3. Add connection tracking/heartbeat - 3 days
4. Add port forwarding - 2 days
5. Full integration testing - 2 days

---

## Current Status

```
┌─────────────────────────────────────┐
│  What We Built (Unused)             │
│  ✅ Transport interface             │
│  ✅ HTTP/stdio transports           │
│  ✅ Fallback mechanism              │
│  ✅ Connection pooling              │
│  ✅ CLI flags                       │
│  ✅ setupTransport() function       │
└─────────────────────────────────────┘
              │
              │ NOT CONNECTED
              ▼
┌─────────────────────────────────────┐
│  What Actually Runs                 │
│  ✅ stdio tunnel client (hardcoded) │
│  ✅ credentials.RunCredentialsServer│
│  ❌ No HTTP tunnel usage            │
└─────────────────────────────────────┘
```

---

## Recommendation

### Option 1: Complete Minimal Integration (2-3 days)
Make our code actually work with minimal changes.

**Pros:**
- Uses what we built
- Simple, maintainable
- No external dependencies

**Cons:**
- Still missing Tailscale
- Not aligned with PR #1836

### Option 2: Abandon and Use PR #1836 (0 days)
Discard our implementation, wait for PR #1836 to merge.

**Pros:**
- Full feature set
- Aligned with upstream
- No additional work

**Cons:**
- Wasted effort
- PR may not merge

### Option 3: Hybrid (1 week)
Keep our transport layer, add minimal server side.

**Pros:**
- Balanced approach
- Reuses our work
- Simpler than full PR

**Cons:**
- Still not full PR #1836
- May need rework later

---

## Bottom Line

**We built a transport layer that isn't connected to anything.**

It's like building a car engine without connecting it to the wheels - technically excellent, but doesn't move the car.

To make it work, we need:
1. Server side (client machine)
2. Protocol definition
3. Integration with credentials server
4. End-to-end testing

**Current state:** 305 lines of high-quality, well-tested, unused code.
