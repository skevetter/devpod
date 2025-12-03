//go:build !linux

package workspace

// RunProcessReaper is a no-op on non-Linux platforms
func RunProcessReaper() {}
