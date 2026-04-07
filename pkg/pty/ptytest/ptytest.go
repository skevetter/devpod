package ptytest

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/skevetter/devpod/pkg/pty"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
)

// ansiEscape matches ANSI/VT100 escape sequences for stripping from log output.
var ansiEscape = regexp.MustCompile(
	"[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|" +
		"(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))",
)

// Timeout constants inlined from coder/testutil.
const (
	waitShort  = 10 * time.Second
	waitMedium = 15 * time.Second
)

func New(t *testing.T, opts ...pty.Option) *PTY {
	t.Helper()

	ptty, err := newTestPTY(opts...)
	require.NoError(t, err)

	e := newExpecter(t, ptty.Output(), "cmd")
	r := &PTY{
		outExpecter: e,
		PTY:         ptty,
	}
	t.Cleanup(func() {
		_ = r.Close()
	})
	return r
}

// Start starts a new process asynchronously and returns a PTYCmd and Process.
// It kills the process and PTYCmd upon cleanup.
func Start(t *testing.T, cmd *pty.Cmd, opts ...pty.StartOption) (*PTYCmd, pty.Process) {
	t.Helper()

	ptty, ps, err := pty.Start(cmd, opts...)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = ps.Kill()
		_ = ps.Wait()
	})
	ex := newExpecter(t, ptty.OutputReader(), cmd.Args[0])

	r := &PTYCmd{
		outExpecter: ex,
		PTYCmd:      ptty,
	}
	t.Cleanup(func() {
		_ = r.Close()
	})
	return r, ps
}

//nolint:funlen // Test helper with necessary setup/teardown plumbing.
func newExpecter(t *testing.T, r io.Reader, name string) outExpecter {
	logDone := make(chan struct{})
	logr, logw := io.Pipe()

	copyDone := make(chan struct{})
	out := newStdbuf()
	w := io.MultiWriter(logw, out)

	ex := outExpecter{
		t:          t,
		out:        out,
		name:       atomic.NewString(name),
		runeReader: bufio.NewReaderSize(out, utf8.UTFMax),
	}

	logClose := func(name string, c io.Closer) {
		ex.logf("closing %s", name)
		err := c.Close()
		ex.logf("closed %s: %v", name, err)
	}

	ex.close = func(reason string) error {
		ctx, cancel := context.WithTimeout(context.Background(), waitShort)
		defer cancel()

		ex.logf("closing expecter: %s", reason)

		select {
		case <-ctx.Done():
			ex.fatalf("close", "copy did not close in time")
		case <-copyDone:
		}

		logClose("logw", logw)
		logClose("logr", logr)
		select {
		case <-ctx.Done():
			ex.fatalf("close", "log pipe did not close in time")
		case <-logDone:
		}

		ex.logf("closed expecter")
		return nil
	}

	go func() {
		defer close(copyDone)
		_, err := io.Copy(w, r)
		ex.logf("copy done: %v", err)
		ex.logf("closing out")
		err = out.closeErr(err)
		ex.logf("closed out: %v", err)
	}()

	go drainLog(&ex, logr, logDone)

	return ex
}

// drainLog reads lines from logr and logs them. Uses bufio.NewReader instead
// of bufio.Scanner to avoid the 64KiB default token limit that causes Scanner
// to stop on long lines.
func drainLog(ex *outExpecter, logr io.Reader, done chan struct{}) {
	defer close(done)
	r := bufio.NewReader(logr)
	for {
		line, err := r.ReadString('\n')
		if line != "" {
			ex.logf("%q", ansiEscape.ReplaceAllString(strings.TrimRight(line, "\n"), ""))
		}
		if err != nil {
			return
		}
	}
}

type outExpecter struct {
	t     *testing.T
	close func(reason string) error
	out   *stdbuf
	name  *atomic.String

	runeReader *bufio.Reader
}

// ExpectMatch uses a background context with a medium timeout.
//
// Deprecated: use ExpectMatchContext instead.
func (e *outExpecter) ExpectMatch(str string) string {
	e.t.Helper()

	timeout, cancel := context.WithTimeout(context.Background(), waitMedium)
	defer cancel()

	return e.ExpectMatchContext(timeout, str)
}

// ExpectRegexMatch uses a background context with a medium timeout.
func (e *outExpecter) ExpectRegexMatch(str string) string {
	e.t.Helper()

	timeout, cancel := context.WithTimeout(context.Background(), waitMedium)
	defer cancel()

	return e.ExpectRegexMatchContext(timeout, str)
}

// ExpectMatchContext reads output until str is found or ctx expires.
func (e *outExpecter) ExpectMatchContext(ctx context.Context, str string) string {
	return e.expectMatcherFunc(ctx, str, strings.Contains)
}

// ExpectRegexMatchContext reads output until pattern matches or ctx expires.
func (e *outExpecter) ExpectRegexMatchContext(ctx context.Context, str string) string {
	return e.expectMatcherFunc(ctx, str, func(src, pattern string) bool {
		return regexp.MustCompile(pattern).MatchString(src)
	})
}

// ExpectNoMatchBefore validates that `match` does not occur before `before`.
func (e *outExpecter) ExpectNoMatchBefore(ctx context.Context, match, before string) string {
	e.t.Helper()

	var buffer bytes.Buffer
	err := e.doMatchWithDeadline(ctx, "ExpectNoMatchBefore", func(rd *bufio.Reader) error {
		for {
			r, _, err := rd.ReadRune()
			if err != nil {
				return err
			}
			_, err = buffer.WriteRune(r)
			if err != nil {
				return err
			}

			if strings.Contains(buffer.String(), match) {
				return fmt.Errorf("found %q before %q", match, before)
			}

			if strings.Contains(buffer.String(), before) {
				return nil
			}
		}
	})
	if err != nil {
		e.fatalf(
			"read error",
			"%v (wanted no %q before %q; got %q)",
			err, match, before, buffer.String(),
		)
		return ""
	}
	e.logf("matched %q = %q", before, ansiEscape.ReplaceAllString(buffer.String(), ""))
	return buffer.String()
}

// Peek returns the next n bytes without advancing the reader.
func (e *outExpecter) Peek(ctx context.Context, n int) []byte {
	e.t.Helper()

	var out []byte
	err := e.doMatchWithDeadline(ctx, "Peek", func(rd *bufio.Reader) error {
		var err error
		out, err = rd.Peek(n)
		return err
	})
	if err != nil {
		e.fatalf("read error", "%v (wanted %d bytes; got %d: %q)", err, n, len(out), out)
		return nil
	}
	e.logf("peeked %d/%d bytes = %q", len(out), n, out)
	return slices.Clone(out)
}

// ReadRune reads a single rune from the output.
//
//nolint:govet // We don't care about conforming to ReadRune() (rune, int, error).
func (e *outExpecter) ReadRune(ctx context.Context) rune {
	e.t.Helper()

	var r rune
	err := e.doMatchWithDeadline(ctx, "ReadRune", func(rd *bufio.Reader) error {
		var err error
		r, _, err = rd.ReadRune()
		return err
	})
	if err != nil {
		e.fatalf("read error", "%v (wanted rune; got %q)", err, r)
		return 0
	}
	e.logf("matched rune = %q", r)
	return r
}

// ReadLine reads output until a newline is encountered.
func (e *outExpecter) ReadLine(ctx context.Context) string {
	e.t.Helper()

	var buffer bytes.Buffer
	err := e.doMatchWithDeadline(ctx, "ReadLine", func(rd *bufio.Reader) error {
		for {
			r, _, err := rd.ReadRune()
			if err != nil {
				return err
			}
			if r == '\n' || r == '\r' {
				if r == '\r' {
					consumeOptionalLF(rd)
				}
				return nil
			}
			_, err = buffer.WriteRune(r)
			if err != nil {
				return err
			}
		}
	})
	if err != nil {
		e.fatalf("read error", "%v (wanted newline; got %q)", err, buffer.String())
		return ""
	}
	e.logf("matched newline = %q", buffer.String())
	return buffer.String()
}

// consumeOptionalLF reads and discards a '\n' if it immediately follows a '\r'.
func consumeOptionalLF(rd *bufio.Reader) {
	b, _ := rd.Peek(1)
	if len(b) > 0 {
		if r, _ := utf8.DecodeRune(b); r == '\n' {
			_, _, _ = rd.ReadRune()
		}
	}
}

// ReadAll returns all buffered output.
func (e *outExpecter) ReadAll() []byte {
	e.t.Helper()
	return e.out.ReadAll()
}

func (e *outExpecter) expectMatcherFunc(
	ctx context.Context,
	str string,
	fn func(src, pattern string) bool,
) string {
	e.t.Helper()

	var buffer bytes.Buffer
	err := e.doMatchWithDeadline(ctx, "ExpectMatchContext", func(rd *bufio.Reader) error {
		for {
			r, _, err := rd.ReadRune()
			if err != nil {
				return err
			}
			_, err = buffer.WriteRune(r)
			if err != nil {
				return err
			}
			if fn(buffer.String(), str) {
				return nil
			}
		}
	})
	if err != nil {
		e.fatalf("read error", "%v (wanted %q; got %q)", err, str, buffer.String())
		return ""
	}
	e.logf("matched %q = %q", str, buffer.String())
	return buffer.String()
}

func (e *outExpecter) doMatchWithDeadline(
	ctx context.Context,
	name string,
	fn func(*bufio.Reader) error,
) error {
	e.t.Helper()

	if _, ok := ctx.Deadline(); !ok {
		timeout := waitMedium
		e.logf("%s ctx has no deadline, using %s", name, timeout)
		var cancel context.CancelFunc
		//nolint:gocritic // Rule guard doesn't detect that we're using wait constants.
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	match := make(chan error, 1)
	go func() {
		defer close(match)
		match <- fn(e.runeReader)
	}()
	select {
	case err := <-match:
		return err
	case <-ctx.Done():
		_ = e.out.Close()
		<-match
		return fmt.Errorf("match deadline exceeded: %w", ctx.Err())
	}
}

func (e *outExpecter) logf(format string, args ...any) {
	e.t.Helper()

	e.t.Logf(
		"%s: %s: %s",
		time.Now().UTC().Format("2006-01-02 15:04:05.000"),
		e.name.Load(),
		fmt.Sprintf(format, args...),
	)
}

func (e *outExpecter) fatalf(reason string, format string, args ...any) {
	e.t.Helper()

	e.logf("%s: %s", reason, fmt.Sprintf(format, args...))
	require.FailNowf(e.t, reason, format, args...)
}

// PTY wraps a pty.PTY with expect-style output matching for tests.
type PTY struct {
	outExpecter
	pty.PTY
	closeOnce sync.Once
	closeErr  error
}

// Close closes the PTY and its expecter.
func (p *PTY) Close() error {
	p.t.Helper()
	p.closeOnce.Do(func() {
		pErr := p.PTY.Close()
		if pErr != nil {
			p.logf("PTY: Close failed: %v", pErr)
		}
		eErr := p.close("PTY close")
		if eErr != nil {
			p.logf("PTY: close expecter failed: %v", eErr)
		}
		if pErr != nil {
			p.closeErr = pErr
			return
		}
		p.closeErr = eErr
	})
	return p.closeErr
}

// Write writes a single rune to the PTY input.
func (p *PTY) Write(r rune) {
	p.t.Helper()

	p.logf("stdin: %q", r)
	_, err := p.Input().Write([]byte{byte(r)}) //nolint:gosec // G115
	require.NoError(p.t, err, "write failed")
}

// WriteLine writes a string followed by a carriage return to the PTY input.
func (p *PTY) WriteLine(str string) {
	p.t.Helper()

	newline := []byte{'\r'}
	if runtime.GOOS == "windows" {
		newline = append(newline, '\n')
	}
	p.logf("stdin: %q", str+string(newline))
	_, err := p.Input().Write(append([]byte(str), newline...))
	require.NoError(p.t, err, "write line failed")
}

// Named sets the PTY name in the logs. Defaults to "cmd".
func (p *PTY) Named(name string) *PTY {
	p.name.Store(name)
	return p
}

// PTYCmd wraps a pty.PTYCmd with expect-style output matching for tests.
type PTYCmd struct {
	outExpecter
	pty.PTYCmd
}

// Close closes the PTYCmd and its expecter.
func (p *PTYCmd) Close() error {
	p.t.Helper()
	pErr := p.PTYCmd.Close()
	if pErr != nil {
		p.logf("PTYCmd: Close failed: %v", pErr)
	}
	eErr := p.close("PTYCmd close")
	if eErr != nil {
		p.logf("PTYCmd: close expecter failed: %v", eErr)
	}
	if pErr != nil {
		return pErr
	}
	return eErr
}

// stdbuf is like a buffered stdout, it buffers writes until read.
type stdbuf struct {
	r io.Reader

	mu   sync.Mutex // Protects following.
	b    []byte
	more chan struct{}
	err  error
}

func newStdbuf() *stdbuf {
	return &stdbuf{more: make(chan struct{}, 1)}
}

// ReadAll returns all buffered data, even if the writer has errored.
// This ensures callers can drain remaining bytes after an error before
// observing the terminal condition on subsequent reads.
func (b *stdbuf) ReadAll() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.b) == 0 {
		return nil
	}
	p := append([]byte(nil), b.b...)
	b.b = b.b[:0]
	return p
}

// Read reads from the buffer, blocking if empty until data is written.
func (b *stdbuf) Read(p []byte) (int, error) {
	if b.r == nil {
		return b.readOrWaitForMore(p)
	}

	n, err := b.r.Read(p)
	if err == io.EOF {
		b.r = nil
		err = nil
		if n == 0 {
			return b.readOrWaitForMore(p)
		}
	}
	return n, err
}

// Write appends data to the buffer.
func (b *stdbuf) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.err != nil {
		return 0, b.err
	}

	b.b = append(b.b, p...)

	select {
	case b.more <- struct{}{}:
	default:
	}

	return len(p), nil
}

// Close signals EOF to readers.
func (b *stdbuf) Close() error {
	return b.closeErr(nil)
}

func (b *stdbuf) readOrWaitForMore(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	select {
	case <-b.more:
	default:
	}

	if len(b.b) == 0 {
		if b.err != nil {
			return 0, b.err
		}

		b.mu.Unlock()
		<-b.more
		b.mu.Lock()
	}

	b.r = bytes.NewReader(b.b)
	b.b = b.b[len(b.b):]

	return b.r.Read(p)
}

func (b *stdbuf) closeErr(err error) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.err != nil {
		return err
	}
	if err == nil {
		b.err = io.EOF
	} else {
		b.err = err
	}
	close(b.more)
	return err
}
