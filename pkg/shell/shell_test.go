package shell

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"
)

func TestRunEmulatedShell_KillExecutesRealBinary(t *testing.T) {
	// "kill -0 PID" checks whether a signal can be sent to a process
	// without actually sending one. It exits 0 if the process exists.
	// This verifies that `kill` invokes the real /bin/kill binary
	// rather than being swallowed by the shell interpreter's builtin stub.
	pid := os.Getpid()
	cmd := fmt.Sprintf("kill -0 %d", pid)

	var stdout, stderr bytes.Buffer
	err := RunEmulatedShell(context.Background(), cmd, nil, &stdout, &stderr, os.Environ())
	if err != nil {
		t.Fatalf(
			"kill -0 on own PID should succeed, got error: %v\nstderr: %s",
			err,
			stderr.String(),
		)
	}
}
