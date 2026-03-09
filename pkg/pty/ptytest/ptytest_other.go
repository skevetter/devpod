//go:build !windows

package ptytest

import "github.com/skevetter/devpod/pkg/pty"

func newTestPTY(opts ...pty.Option) (pty.PTY, error) {
	return pty.New(opts...)
}
