# Implementation Plan: Full Network Proxy (PR #1836 Feature Parity)

**Date:** 2025-12-04
**Goal:** Implement full network proxy functionality from PR #1836
**Target Coverage:** 80% minimum
**Testing:** testify suite for unit tests, e2e/tests for integration

---

## Current State

### What We Have ✅
- Basic HTTP transport layer (305 lines)
- HTTP tunnel server (124 lines)
- Transport abstraction interface
- Stdio fallback mechanism
- CLI integration
- 56 tests, 64% coverage

### What We Need ❌
Based on PR #1836 analysis:

1. **Connection Management**
   - Connection tracker (track active connections)
   - Heartbeat system (monitor connection health)
   - Connection lifecycle management

2. **Network Services**
   - gRPC proxy (reverse proxy with director)
   - HTTP proxy handler (full proxy, not just dial)
   - Port forwarding service
   - SSH tunneling service

3. **Network Infrastructure**
   - Network server coordination
   - Network client helpers
   - Network mapping (peer discovery)
   - Connection multiplexing (cmux)

4. **Platform Integration**
   - Platform credentials server
   - Workspace daemon refactor
   - Network utilities

5. **Dependencies**
   - `github.com/mwitkow/grpc-proxy` - gRPC reverse proxy
   - `github.com/soheilhy/cmux` - Connection multiplexing
   - Note: Skipping Tailscale (tsnet) for now - can add later

---

## Implementation Strategy

### Phase 1: Core Infrastructure (Days 1-2)
1. Connection tracker
2. Heartbeat system
3. Network utilities
4. Update dependencies

### Phase 2: Proxy Services (Days 3-4)
5. gRPC proxy
6. HTTP proxy handler (upgrade from simple dial)
7. Platform credentials server

### Phase 3: Additional Services (Days 5-6)
8. Port forwarding service
9. SSH tunneling service
10. Network mapping

### Phase 4: Integration (Day 7)
11. Network server coordination
12. Network client helpers
13. Workspace daemon integration
14. Full E2E testing

### Phase 5: Testing & Documentation (Day 8)
15. Achieve 80% coverage
16. E2E integration tests
17. Documentation updates

---

## File-by-File Implementation Plan

### New Files to Create

#### Phase 1: Core Infrastructure
```
pkg/daemon/workspace/network/
├── connection_tracker.go      - Track active connections
├── connection_tracker_test.go
├── heartbeat.go               - Connection health monitoring
├── heartbeat_test.go
├── util.go                    - Network utilities
└── util_test.go
```

#### Phase 2: Proxy Services
```
pkg/daemon/workspace/network/
├── grpc_proxy.go              - gRPC reverse proxy
├── grpc_proxy_test.go
├── http_proxy.go              - Full HTTP proxy handler (upgrade existing)
├── http_proxy_test.go
├── platform_credentials_server.go - Platform credential handling
└── platform_credentials_server_test.go

pkg/daemon/local/
├── grpc_proxy.go              - Client-side gRPC proxy
└── grpc_proxy_test.go
```

#### Phase 3: Additional Services
```
pkg/daemon/workspace/network/
├── port_forward.go            - Port forwarding service
├── port_forward_test.go
├── ssh.go                     - SSH tunneling
├── ssh_test.go
├── netmap.go                  - Network mapping
└── netmap_test.go
```

#### Phase 4: Integration
```
pkg/daemon/workspace/network/
├── server.go                  - Network server coordination
├── server_test.go
├── client.go                  - Network client helpers
└── client_test.go

pkg/daemon/workspace/
├── daemon.go                  - Workspace daemon (refactored)
├── daemon_test.go
├── network.go                 - Network integration
└── network_test.go

e2e/tests/network/
├── full_proxy_test.go         - Full proxy E2E tests
├── port_forward_test.go       - Port forwarding E2E
└── ssh_tunnel_test.go         - SSH tunnel E2E
```

### Files to Modify
```
cmd/agent/container/credentials_server.go  - Enhanced integration
cmd/agent/container/daemon.go              - Move to pkg/daemon/workspace/
go.mod                                     - Add dependencies
go.sum                                     - Update checksums
```

---

## Dependencies to Add

```go
// go.mod additions
require (
    github.com/mwitkow/grpc-proxy v0.0.0-20230212185441-f345521cb9c9
    github.com/soheilhy/cmux v0.1.5
)
```

---

## Testing Strategy

### Unit Tests (testify suite)
- Each new file gets a corresponding `_test.go`
- Use `testify/suite` for test organization
- Mock external dependencies
- Target: 80% coverage per package

### Integration Tests (e2e/tests)
- Full proxy workflow
- Port forwarding scenarios
- SSH tunneling scenarios
- Connection tracking and heartbeat
- Fallback scenarios

### Test Coverage Tracking
```bash
# Current: 64%
# Target: 80%
# Per-file minimum: 75%
```

---

## Implementation Order

### Day 1: Foundation
1. ✅ Review current implementation
2. ✅ Create implementation plan
3. ⬜ Add dependencies (grpc-proxy, cmux)
4. ⬜ Implement connection_tracker.go + tests
5. ⬜ Implement heartbeat.go + tests
6. ⬜ Implement util.go + tests

### Day 2: Core Proxy
7. ⬜ Implement grpc_proxy.go (workspace) + tests
8. ⬜ Implement grpc_proxy.go (local) + tests
9. ⬜ Upgrade http_proxy.go to full handler + tests
10. ⬜ Implement platform_credentials_server.go + tests

### Day 3: Services
11. ⬜ Implement port_forward.go + tests
12. ⬜ Implement ssh.go + tests
13. ⬜ Implement netmap.go + tests

### Day 4: Integration
14. ⬜ Implement server.go + tests
15. ⬜ Implement client.go + tests
16. ⬜ Refactor workspace daemon
17. ⬜ Implement network.go integration

### Day 5: Testing
18. ⬜ E2E tests for full proxy
19. ⬜ E2E tests for port forwarding
20. ⬜ E2E tests for SSH tunneling
21. ⬜ Achieve 80% coverage

### Day 6: Documentation & Polish
22. ⬜ Update documentation
23. ⬜ Code review and cleanup
24. ⬜ Final testing
25. ⬜ Ready for merge

---

## Success Criteria

### Functional Requirements
- ✅ gRPC reverse proxy working
- ✅ HTTP proxy handler working
- ✅ Port forwarding working
- ✅ SSH tunneling working
- ✅ Connection tracking working
- ✅ Heartbeat monitoring working
- ✅ Platform credentials working
- ✅ Backward compatible with stdio

### Quality Requirements
- ✅ 80% test coverage minimum
- ✅ All tests passing
- ✅ No linting errors
- ✅ Clean code (minimal, no bloat)
- ✅ Well documented

### Integration Requirements
- ✅ CLI commands working
- ✅ E2E tests passing
- ✅ Backward compatible
- ✅ Graceful fallback

---

## Risk Mitigation

### Risk 1: Complexity
- **Mitigation:** Implement incrementally, test each piece
- **Fallback:** Keep existing minimal implementation working

### Risk 2: Dependencies
- **Mitigation:** Vendor dependencies, test compatibility
- **Fallback:** Implement minimal versions if needed

### Risk 3: Coverage Target
- **Mitigation:** Write tests alongside code (TDD)
- **Fallback:** Focus on critical paths first

### Risk 4: Breaking Changes
- **Mitigation:** Maintain backward compatibility
- **Fallback:** Feature flags for new functionality

---

## Notes

- Skip Tailscale (tsnet) for now - can add in future phase
- Focus on core proxy functionality first
- Maintain backward compatibility throughout
- Use existing transport layer as foundation
- Keep code minimal and clean

---

## Progress Tracking

**Current Status:** Planning Complete ✅
**Next Step:** Add dependencies and implement connection tracker

**Estimated Timeline:** 6 days
**Start Date:** 2025-12-04
**Target Completion:** 2025-12-10
