//go:build !windows

package pty

import "golang.org/x/term"

type terminalState *term.State

func makeInputRaw(fd uintptr) (*TerminalState, error) {
	s, err := term.MakeRaw(int(fd)) //#nosec G115 -- fd is a valid file descriptor
	if err != nil {
		return nil, err
	}
	return &TerminalState{
		state: s,
	}, nil
}

func makeOutputRaw(_ uintptr) (*TerminalState, error) {
	// Does nothing. makeInputRaw does enough for both input and output.
	return &TerminalState{
		state: nil,
	}, nil
}

func restoreTerminal(fd uintptr, state *TerminalState) error {
	if state == nil || state.state == nil {
		return nil
	}

	return term.Restore(int(fd), state.state) //nolint:gosec // G115
}
