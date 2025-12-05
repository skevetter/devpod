# Network Tests Need Fixing

## Issue Identified

Network integration tests in `e2e/tests/network/` subdirectories are using the old pattern that doesn't work:
- Using `framework.NewDefaultFramework()` directly
- Not using `setupDockerProvider()` helper
- Using `--id` flag with DevPodUp
- Not using `framework.CopyToTempDir()` for testdata

## Files That Need Fixing (10 files)

1. `/e2e/tests/network/connection/lifecycle.go`
2. `/e2e/tests/network/connection/tracking.go`
3. `/e2e/tests/network/integration/credentials.go`
4. `/e2e/tests/network/integration/port_forward.go`
5. `/e2e/tests/network/integration/ssh_tunnel.go`
6. `/e2e/tests/network/integration/traffic.go`
7. `/e2e/tests/network/platform/container.go`
8. `/e2e/tests/network/platform/daemon.go`
9. `/e2e/tests/network/platform/kubernetes.go`
10. `/e2e/tests/network/proxy/server.go`

## Working Pattern (from commands/ping.go)

```go
ginkgo.It("test name", func() {
    ctx := context.Background()
    f, err := setupDockerProvider(initialDir + "/bin")  // Use helper
    framework.ExpectNoError(err)

    tempDir, err := framework.CopyToTempDir("tests/network/testdata/simple-app")
    framework.ExpectNoError(err)
    ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

    err = f.DevPodUp(ctx, tempDir)  // No --id flag
    framework.ExpectNoError(err)

    out, err := f.DevPodSSH(ctx, tempDir, "command")  // Use tempDir, not name
    framework.ExpectNoError(err)
})
```

## Old Pattern (broken)

```go
ginkgo.It("test name", func() {
    ctx := context.Background()
    f := framework.NewDefaultFramework(initialDir + "/../../bin")  // Wrong

    _ = f.DevPodProviderDelete(ctx, "docker")  // Manual setup
    err := f.DevPodProviderAdd(ctx, "docker")
    framework.ExpectNoError(err)
    err = f.DevPodProviderUse(ctx, "docker")
    framework.ExpectNoError(err)

    testDir := filepath.Join(initialDir, "testdata", "simple-app")  // Direct path
    name := "test-something"
    ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), name)

    err = f.DevPodUp(ctx, testDir, "--id", name)  // Using --id
    framework.ExpectNoError(err)

    out, err := f.DevPodSSH(ctx, name, "command")  // Using name
    framework.ExpectNoError(err)
})
```

## Changes Needed

### 1. Add helper.go to each subdirectory
Already created:
- ✅ `network/connection/helper.go`
- ✅ `network/integration/helper.go`
- ✅ `network/platform/helper.go`
- ✅ `network/proxy/helper.go`

### 2. Update each test file

For each file, replace:

**Old**:
```go
f := framework.NewDefaultFramework(initialDir + "/../../bin")
_ = f.DevPodProviderDelete(ctx, "docker")
err := f.DevPodProviderAdd(ctx, "docker")
framework.ExpectNoError(err)
err = f.DevPodProviderUse(ctx, "docker")
framework.ExpectNoError(err)
```

**New**:
```go
f, err := setupDockerProvider(initialDir + "/../../../bin")
framework.ExpectNoError(err)
```

**Old**:
```go
testDir := filepath.Join(initialDir, "testdata", "simple-app")
name := "test-something"
ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), name)
```

**New**:
```go
tempDir, err := framework.CopyToTempDir("tests/network/testdata/simple-app")
framework.ExpectNoError(err)
ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)
```

**Old**:
```go
err = f.DevPodUp(ctx, testDir, "--id", name)
```

**New**:
```go
err = f.DevPodUp(ctx, tempDir)
```

**Old**:
```go
f.DevPodSSH(ctx, name, "command")
```

**New**:
```go
f.DevPodSSH(ctx, tempDir, "command")
```

## Status

- ✅ Commands tests fixed and working (2 tests passing)
- ✅ Helper functions created for network subdirectories
- ❌ Network test files need manual fixing (sed automation failed)
- ❌ 10 files with compilation errors

## Recommendation

Manually fix each of the 10 network test files following the working pattern from `commands/ping.go` and `commands/agent.go`.

## Test Validation

After fixing, run:
```bash
cd e2e
ginkgo --label-filter="lifecycle" .  # Test one
ginkgo --label-filter="network:connection" .  # Test connection suite
ginkgo --label-filter="network:integration" .  # Test integration suite
```
