//go:build windows

package pty

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32                = windows.NewLazySystemDLL("kernel32.dll")
	procResizePseudoConsole = kernel32.NewProc("ResizePseudoConsole")
	procCreatePseudoConsole = kernel32.NewProc("CreatePseudoConsole")
	procClosePseudoConsole  = kernel32.NewProc("ClosePseudoConsole")
)

// See: https://docs.microsoft.com/en-us/windows/console/creating-a-pseudoconsole-session
func newPty(opt ...Option) (PTY, error) {
	var opts ptyOptions
	for _, o := range opt {
		o(&opts)
	}

	// We use the CreatePseudoConsole API which was introduced in build 17763
	vsn := windows.RtlGetVersion()
	if vsn.MajorVersion < 10 ||
		vsn.BuildNumber < 17763 {
		return nil, fmt.Errorf("pty not supported")
	}

	// On Windows, pty.New() without Start() is only used by ptytest.New() for
	// in-process CLI testing. ConPTY requires an attached process to function
	// correctly, so ptytest has its own pipe-based implementation. Production
	// code should use pty.Start() which creates a ConPTY with process attached.
	return nil, fmt.Errorf(
		"pty without process not supported on Windows; use ptytest.New() for tests",
	)
}

// newConPty creates a PTY backed by a Windows PseudoConsole (ConPTY). This
// should only be used when a process will be attached via Start().
func newConPty(opt ...Option) (*ptyWindows, error) {
	// CreatePseudoConsole requires Windows 10 version 1809 (build 17763) or later.
	// https://learn.microsoft.com/en-us/windows/console/createpseudoconsole#requirements
	vsn := windows.RtlGetVersion()
	if vsn.MajorVersion < 10 || vsn.BuildNumber < 17763 {
		return nil, fmt.Errorf("ConPTY not supported (build %d < 17763)", vsn.BuildNumber)
	}

	var opts ptyOptions
	for _, o := range opt {
		o(&opts)
	}

	pty := &ptyWindows{
		opts: opts,
		// Initialize to InvalidHandle so closeConsoleNoLock's guard check
		// skips ClosePseudoConsole if CreatePseudoConsole never succeeded.
		console: windows.InvalidHandle,
	}

	var err error
	pty.inputRead, pty.inputWrite, err = os.Pipe()
	if err != nil {
		return nil, err
	}
	pty.outputRead, pty.outputWrite, err = os.Pipe()
	if err != nil {
		_ = pty.inputRead.Close()
		_ = pty.inputWrite.Close()
		return nil, err
	}

	// Default dimensions. Capped at 32767 because COORD uses SHORT (signed 16-bit).
	// https://learn.microsoft.com/en-us/windows/console/coord-str
	width, height := 80, 80
	if opts.sshReq != nil {
		if w := opts.sshReq.Window.Width; w > 0 && w <= 32767 {
			width = w
		}
		if h := opts.sshReq.Window.Height; h > 0 && h <= 32767 {
			height = h
		}
	}

	consoleSize := uintptr(width) + (uintptr(height) << 16)

	ret, _, err := procCreatePseudoConsole.Call(
		consoleSize,
		uintptr(pty.inputRead.Fd()),
		uintptr(pty.outputWrite.Fd()),
		0,
		uintptr(unsafe.Pointer(&pty.console)),
	)
	// CreatePseudoConsole returns S_OK on success, as per:
	// https://learn.microsoft.com/en-us/windows/console/createpseudoconsole
	if windows.Handle(ret) != windows.S_OK {
		_ = pty.Close()
		return nil, fmt.Errorf("create pseudo console (%d): %w", int32(ret), err)
	}

	return pty, nil
}

type ptyWindows struct {
	opts    ptyOptions
	console windows.Handle

	outputWrite *os.File
	outputRead  *os.File
	inputWrite  *os.File
	inputRead   *os.File

	closeMutex sync.Mutex
	closed     bool
}

type windowsProcess struct {
	// cmdDone protects access to cmdErr: anything reading cmdErr should read from cmdDone first.
	cmdDone chan any
	cmdErr  error
	proc    *os.Process
	pw      *ptyWindows
}

// Name returns the TTY name on Windows.
//
// Not implemented.
func (p *ptyWindows) Name() string {
	return ""
}

func (p *ptyWindows) Output() ReadWriter {
	return ReadWriter{
		Reader: p.outputRead,
		Writer: p.outputWrite,
	}
}

func (p *ptyWindows) OutputReader() io.Reader {
	return p.outputRead
}

func (p *ptyWindows) Input() ReadWriter {
	return ReadWriter{
		Reader: p.inputRead,
		Writer: p.inputWrite,
	}
}

func (p *ptyWindows) InputWriter() io.Writer {
	return p.inputWrite
}

func (p *ptyWindows) Resize(height uint16, width uint16) error {
	// hold the lock, so we don't race with anyone trying to close the console
	p.closeMutex.Lock()
	defer p.closeMutex.Unlock()
	if p.closed || p.console == windows.InvalidHandle {
		return ErrClosed
	}
	// COORD fields are SHORT (signed 16-bit, max 32767). Values above this
	// wrap negative and produce invalid dimensions.
	// https://learn.microsoft.com/en-us/windows/console/coord-str
	// https://learn.microsoft.com/en-us/windows/win32/winprog/windows-data-types
	if height > 32767 || width > 32767 {
		return fmt.Errorf("pty: dimensions %dx%d exceed maximum (32767)", width, height)
	}
	// Taken from: https://github.com/microsoft/hcsshim/blob/54a5ad86808d761e3e396aff3e2022840f39f9a8/internal/winapi/zsyscall_windows.go#L144
	ret, _, err := procResizePseudoConsole.Call(
		uintptr(p.console),
		uintptr(*((*uint32)(unsafe.Pointer(&windows.Coord{
			Y: int16(height),
			X: int16(width),
		})))),
	)
	if windows.Handle(ret) != windows.S_OK {
		return err
	}
	return nil
}

// closeConsoleNoLock closes the console handle, and sets it to
// windows.InvalidHandle. It must be called with p.closeMutex held.
func (p *ptyWindows) closeConsoleNoLock() error {
	if p.console != windows.InvalidHandle {
		ret, _, err := procClosePseudoConsole.Call(uintptr(p.console))
		if winerrorFailed(ret) {
			return fmt.Errorf("close pseudo console (%d): %w", ret, err)
		}
		p.console = windows.InvalidHandle
	}

	return nil
}

func (p *ptyWindows) Close() error {
	p.closeMutex.Lock()
	defer p.closeMutex.Unlock()
	if p.closed {
		return nil
	}

	err := p.closeConsoleNoLock()
	if err != nil {
		return err
	}

	p.closed = true

	if p.outputWrite != nil {
		_ = p.outputWrite.Close()
	}
	_ = p.outputRead.Close()
	_ = p.inputWrite.Close()
	if p.inputRead != nil {
		_ = p.inputRead.Close()
	}
	return nil
}

func (p *windowsProcess) waitInternal() {
	defer close(p.cmdDone)
	defer func() {
		p.pw.closeMutex.Lock()
		defer p.pw.closeMutex.Unlock()

		err := p.pw.closeConsoleNoLock()
		if err != nil && p.cmdErr == nil {
			p.cmdErr = err
		}
	}()

	state, err := p.proc.Wait()
	if err != nil {
		p.cmdErr = err
		return
	}
	if !state.Success() {
		p.cmdErr = &exec.ExitError{ProcessState: state}
		return
	}
}

func (p *windowsProcess) Wait() error {
	<-p.cmdDone
	return p.cmdErr
}

func (p *windowsProcess) Kill() error {
	return p.proc.Kill()
}

func (p *windowsProcess) Signal(sig os.Signal) error {
	// Windows doesn't support signals.
	return p.Kill()
}

// killOnContext waits for the context to be done and kills the process, unless it exits on its own first.
func (p *windowsProcess) killOnContext(ctx context.Context) {
	select {
	case <-p.cmdDone:
		return
	case <-ctx.Done():
		p.Kill()
	}
}

// winerrorFailed returns true if the syscall failed, this function
// assumes the return value is a 32-bit integer, like HRESULT.
func winerrorFailed(r1 uintptr) bool {
	return int32(r1) < 0
}
