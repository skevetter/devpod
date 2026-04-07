//go:build !linux

package agent

import "context"

// RunProcessReaper is a no-op on non-Linux platforms.
func RunProcessReaper(_ context.Context) {}
