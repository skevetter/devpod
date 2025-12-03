# Network Proxy Documentation

## Overview

DevPod's network proxy system provides advanced networking capabilities for workspace containers, including port forwarding, SSH tunneling, gRPC/HTTP proxying, and connection management.

## Features

### Core Features
- **Connection Tracking**: Monitor and manage active connections
- **Heartbeat Monitoring**: Automatically remove stale connections
- **Network Mapping**: Peer discovery and management

### Proxy Services
- **gRPC Proxy**: Reverse proxy for gRPC services
- **HTTP Proxy**: Full HTTP proxy with connection hijacking
- **Platform Credentials**: Secure git/docker credential forwarding

### Tunneling Services
- **Port Forwarding**: Bidirectional TCP port forwarding
- **SSH Tunneling**: Secure SSH-style tunnels

## Usage

### 1. Network Proxy Server

Start a network proxy server with gRPC and HTTP proxying:

```bash
devpod agent container network-proxy \
  --addr localhost:9090 \
  --grpc-target localhost:50051 \
  --http-target localhost:8080
```

**Flags:**
- `--addr` - Address to listen on (default: `localhost:9090`)
- `--grpc-target` - gRPC target address (optional)
- `--http-target` - HTTP target address (optional)

### 2. Port Forwarding

Forward a local port to a remote address:

```bash
devpod agent container port-forward \
  --local-port 8080 \
  --remote-addr api.example.com:80
```

**Flags:**
- `--local-port` - Local port to forward (required)
- `--remote-addr` - Remote address in host:port format (required)

**Example:**
```bash
# Forward local port 3000 to remote database
devpod agent container port-forward \
  --local-port 3000 \
  --remote-addr db.internal:5432

# Access via localhost:3000
psql -h localhost -p 3000 -U user database
```

### 3. SSH Tunnel

Create an SSH-style tunnel:

```bash
devpod agent container ssh-tunnel \
  --local-addr localhost:2222 \
  --remote-addr ssh-server:22
```

**Flags:**
- `--local-addr` - Local address to bind (default: `localhost:0` for random port)
- `--remote-addr` - Remote address in host:port format (required)

**Example:**
```bash
# Create tunnel with specific local port
devpod agent container ssh-tunnel \
  --local-addr localhost:2222 \
  --remote-addr bastion:22

# Connect via tunnel
ssh -p 2222 user@localhost
```

## Configuration

### Daemon Configuration

Configure network proxy in daemon config:

```json
{
  "networkProxy": {
    "enabled": true,
    "addr": "localhost:9090",
    "grpcTarget": "localhost:50051",
    "httpTarget": "localhost:8080"
  }
}
```

Place configuration in:
- File: `/var/run/secrets/devpod/daemon_config`
- Environment: `DEVPOD_WORKSPACE_DAEMON_CONFIG`

### Environment Variables

- `DEVPOD_HTTP_TUNNEL_CLIENT` - HTTP tunnel client address
- `DEVPOD_WORKSPACE_DAEMON_CONFIG` - Daemon configuration

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│  Client Machine                                             │
│  ┌───────────────────────────────────────────────────────┐  │
│  │  gRPC Proxy Server                                    │  │
│  │  - Reverse proxy for gRPC                             │  │
│  └───────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
                           │
                           │ Network
                           │
┌─────────────────────────────────────────────────────────────┐
│  Workspace Container                                        │
│  ┌───────────────────────────────────────────────────────┐  │
│  │  Network Proxy Server                                 │  │
│  │  ├─ gRPC Proxy                                        │  │
│  │  ├─ HTTP Proxy                                        │  │
│  │  ├─ Connection Tracker                                │  │
│  │  ├─ Heartbeat Monitor                                 │  │
│  │  ├─ Port Forwarder                                    │  │
│  │  └─ Network Map                                       │  │
│  └───────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

## Examples

### Example 1: Database Access

Forward local port to remote database:

```bash
# Start port forward
devpod agent container port-forward \
  --local-port 5432 \
  --remote-addr postgres.internal:5432

# Connect from local machine
psql -h localhost -p 5432 -U user mydb
```

### Example 2: API Development

Forward to internal API:

```bash
# Forward API port
devpod agent container port-forward \
  --local-port 8080 \
  --remote-addr api.internal:80

# Access API locally
curl http://localhost:8080/api/v1/users
```

### Example 3: SSH Bastion

Create tunnel through bastion:

```bash
# Create tunnel
devpod agent container ssh-tunnel \
  --local-addr localhost:2222 \
  --remote-addr bastion.internal:22

# SSH through tunnel
ssh -p 2222 user@localhost
```

## Troubleshooting

### Port Already in Use

```bash
Error: listen tcp :8080: bind: address already in use
```

**Solution:** Use a different port or stop the conflicting service.

### Connection Refused

```bash
Error: dial tcp: connect: connection refused
```

**Solution:** Verify the remote address is correct and the service is running.

### Permission Denied

```bash
Error: listen tcp :22: bind: permission denied
```

**Solution:** Use a port > 1024 or run with appropriate permissions.

## Performance

- **HTTP Latency**: ~0.2ms overhead
- **Connection Pool**: Zero-allocation reuse
- **Heartbeat**: Configurable interval (default: 30s)
- **Timeout**: Configurable (default: 90s)

## Security

- **Localhost Only**: Binds to localhost by default
- **No External Exposure**: Services not exposed externally
- **Credential Security**: Credentials stay on local machine
- **Automatic Cleanup**: Connections cleaned up on exit

## Advanced Usage

### Custom Heartbeat Configuration

```go
config := network.HeartbeatConfig{
    Interval: 30 * time.Second,
    Timeout:  90 * time.Second,
}
```

### Connection Tracking

```go
tracker := network.NewConnectionTracker()
tracker.Add("conn1", "192.168.1.1:8080")
tracker.Update("conn1")
conns := tracker.List()
```

### Network Mapping

```go
netmap := network.NewNetworkMap()
netmap.AddPeer("peer1", "192.168.1.1:8080")
peers := netmap.ListPeers()
```

## API Reference

See the [API documentation](../pkg/daemon/workspace/network/README.md) for detailed API reference.

## Contributing

Contributions are welcome! See [CONTRIBUTING.md](../CONTRIBUTING.md) for guidelines.

## License

See [LICENSE](../LICENSE) for details.
