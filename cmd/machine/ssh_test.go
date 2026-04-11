package machine

import (
	"errors"
	"os"
	"testing"

	"github.com/skevetter/devpod/pkg/pty"
)

type filePair struct {
	reader *os.File
	writer *os.File
}

func TestHasInteractiveTerminalRequiresStdinAndStdout(t *testing.T) {
	stdin := newPipePair(t, "stdin")
	stdout := newPipePair(t, "stdout")
	restore := stubIsTerminalFunc(t)
	defer restore()

	isTerminalFunc = func(fd uintptr) bool {
		return fd == stdin.reader.Fd()
	}
	if hasInteractiveTerminal(stdin.reader, stdout.writer) {
		t.Fatal("expected missing stdout terminal to disable PTY")
	}

	isTerminalFunc = func(fd uintptr) bool {
		return fd == stdout.writer.Fd()
	}
	if hasInteractiveTerminal(stdin.reader, stdout.writer) {
		t.Fatal("expected missing stdin terminal to disable PTY")
	}

	isTerminalFunc = func(fd uintptr) bool {
		return fd == stdin.reader.Fd() || fd == stdout.writer.Fd()
	}
	if !hasInteractiveTerminal(stdin.reader, stdout.writer) {
		t.Fatal("expected PTY when both stdin and stdout are terminals")
	}
}

func TestMakeRawTermUsesInputAndOutputTerminalState(t *testing.T) {
	stdin := newPipePair(t, "stdin")
	stdout := newPipePair(t, "stdout")
	stdinState := &pty.TerminalState{}
	stdoutState := &pty.TerminalState{}
	var calls []string
	restore := stubTerminalFuncs(t)
	defer restore()

	makeInputRawTerm = makeRawRecorder(t, &calls, "input", stdin.reader.Fd(), stdinState, nil)
	makeOutputRawTerm = makeRawRecorder(t, &calls, "output", stdout.writer.Fd(), stdoutState, nil)
	restoreTerminalFunc = restoreRecorder(t, &calls, map[uintptr]restoreCall{
		stdout.writer.Fd(): {state: stdoutState, label: "restore-output"},
		stdin.reader.Fd():  {state: stdinState, label: "restore-input"},
	})

	restoreTerm, err := makeRawTerm(stdin.reader, stdout.writer)
	if err != nil {
		t.Fatalf("make raw term: %v", err)
	}
	restoreTerm()

	assertCallsEqual(t, calls, []string{"input", "output", "restore-output", "restore-input"})
}

func TestMakeRawTermRestoresInputOnOutputFailure(t *testing.T) {
	stdin := newPipePair(t, "stdin")
	stdout := newPipePair(t, "stdout")
	stdinState := &pty.TerminalState{}
	outputErr := errors.New("output raw failed")
	var restoreCalled bool
	restore := stubTerminalFuncs(t)
	defer restore()

	makeInputRawTerm = makeRawRecorder(t, nil, "", stdin.reader.Fd(), stdinState, nil)
	makeOutputRawTerm = makeRawRecorder(t, nil, "", stdout.writer.Fd(), nil, outputErr)
	restoreTerminalFunc = func(fd uintptr, state *pty.TerminalState) error {
		assertRestoreCall(t, fd, state, stdin.reader.Fd(), stdinState)
		restoreCalled = true
		return nil
	}

	restoreTerm, err := makeRawTerm(stdin.reader, stdout.writer)
	if !errors.Is(err, outputErr) {
		t.Fatalf("expected output error, got %v", err)
	}
	if restoreTerm == nil {
		t.Fatal("expected restore function")
	}
	if !restoreCalled {
		t.Fatal("expected stdin terminal state to be restored after output setup failure")
	}
}

func TestMakeRawTermStopsWhenInputSetupFails(t *testing.T) {
	stdin := newPipePair(t, "stdin")
	stdout := newPipePair(t, "stdout")
	inputErr := errors.New("input raw failed")
	var outputCalled bool
	var restoreCalled bool
	restore := stubTerminalFuncs(t)
	defer restore()

	makeInputRawTerm = makeRawRecorder(t, nil, "", stdin.reader.Fd(), nil, inputErr)
	makeOutputRawTerm = func(uintptr) (*pty.TerminalState, error) {
		outputCalled = true
		return &pty.TerminalState{}, nil
	}
	restoreTerminalFunc = func(uintptr, *pty.TerminalState) error {
		restoreCalled = true
		return nil
	}

	restoreTerm, err := makeRawTerm(stdin.reader, stdout.writer)
	if !errors.Is(err, inputErr) {
		t.Fatalf("expected input error, got %v", err)
	}
	if restoreTerm == nil {
		t.Fatal("expected restore function")
	}
	if outputCalled {
		t.Fatal("did not expect output raw setup after input failure")
	}
	if restoreCalled {
		t.Fatal("did not expect terminal restore after input failure")
	}
}

type restoreCall struct {
	state *pty.TerminalState
	label string
}

func newPipePair(t *testing.T, name string) filePair {
	t.Helper()

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create %s pipe: %v", name, err)
	}
	t.Cleanup(func() {
		_ = reader.Close()
		_ = writer.Close()
	})

	return filePair{reader: reader, writer: writer}
}

func stubIsTerminalFunc(t *testing.T) func() {
	t.Helper()

	oldIsTerminalFunc := isTerminalFunc
	t.Cleanup(func() {
		isTerminalFunc = oldIsTerminalFunc
	})

	return func() {
		isTerminalFunc = oldIsTerminalFunc
	}
}

func stubTerminalFuncs(t *testing.T) func() {
	t.Helper()

	oldMakeInputRawTerm := makeInputRawTerm
	oldMakeOutputRawTerm := makeOutputRawTerm
	oldRestoreTerminalFunc := restoreTerminalFunc
	t.Cleanup(func() {
		makeInputRawTerm = oldMakeInputRawTerm
		makeOutputRawTerm = oldMakeOutputRawTerm
		restoreTerminalFunc = oldRestoreTerminalFunc
	})

	return func() {
		makeInputRawTerm = oldMakeInputRawTerm
		makeOutputRawTerm = oldMakeOutputRawTerm
		restoreTerminalFunc = oldRestoreTerminalFunc
	}
}

func makeRawRecorder(
	t *testing.T,
	calls *[]string,
	label string,
	wantFD uintptr,
	state *pty.TerminalState,
	err error,
) func(uintptr) (*pty.TerminalState, error) {
	t.Helper()

	return func(fd uintptr) (*pty.TerminalState, error) {
		if calls != nil && label != "" {
			*calls = append(*calls, label)
		}
		if fd != wantFD {
			t.Fatalf("expected fd %d, got %d", wantFD, fd)
		}
		return state, err
	}
}

func restoreRecorder(
	t *testing.T,
	calls *[]string,
	want map[uintptr]restoreCall,
) func(uintptr, *pty.TerminalState) error {
	t.Helper()

	return func(fd uintptr, state *pty.TerminalState) error {
		call, ok := want[fd]
		if !ok || state != call.state {
			t.Fatalf("unexpected restore call: fd=%d state=%p", fd, state)
		}
		*calls = append(*calls, call.label)
		return nil
	}
}

func assertRestoreCall(
	t *testing.T,
	fd uintptr,
	state *pty.TerminalState,
	wantFD uintptr,
	wantState *pty.TerminalState,
) {
	t.Helper()

	if fd != wantFD || state != wantState {
		t.Fatalf("unexpected restore call: fd=%d state=%p", fd, state)
	}
}

func assertCallsEqual(t *testing.T, got, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("unexpected calls: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected calls: got %v want %v", got, want)
		}
	}
}
