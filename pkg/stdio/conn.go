package stdio

import (
	"bufio"
	"io"
	"net"
	"os"
	"time"
)

// StdioStream is the struct that implements the net.Conn interface.
type StdioStream struct {
	in     *bufio.Reader
	out    *bufio.Writer
	outRaw io.WriteCloser
	local  *StdinAddr
	remote *StdinAddr

	exitOnClose bool
	exitCode    int
}

// NewStdioStream is used to implement the connection interface.
// Uses buffered I/O to prevent terminal escape sequence fragmentation.
func NewStdioStream(in io.Reader, out io.WriteCloser, exitOnClose bool, exitCode int) *StdioStream {
	return &StdioStream{
		local:       NewStdinAddr("local"),
		remote:      NewStdinAddr("remote"),
		in:          bufio.NewReaderSize(in, 32*1024),
		out:         bufio.NewWriterSize(out, 32*1024),
		outRaw:      out,
		exitOnClose: exitOnClose,
		exitCode:    exitCode,
	}
}

// LocalAddr implements interface.
func (s *StdioStream) LocalAddr() net.Addr {
	return s.local
}

// RemoteAddr implements interface.
func (s *StdioStream) RemoteAddr() net.Addr {
	return s.remote
}

// Read implements interface.
func (s *StdioStream) Read(b []byte) (n int, err error) {
	return s.in.Read(b)
}

// Write implements interface.
// Flushes immediately to prevent escape sequence fragmentation.
func (s *StdioStream) Write(b []byte) (n int, err error) {
	n, err = s.out.Write(b)
	if err != nil {
		return n, err
	}
	return n, s.out.Flush()
}

// Close implements interface.
func (s *StdioStream) Close() error {
	if err := s.out.Flush(); err != nil {
		return err
	}

	if s.exitOnClose {
		// We kill ourself here because the streams are closed
		os.Exit(s.exitCode)
	}

	return s.outRaw.Close()
}

// SetDeadline implements interface.
func (s *StdioStream) SetDeadline(t time.Time) error {
	return nil
}

// SetReadDeadline implements interface.
func (s *StdioStream) SetReadDeadline(t time.Time) error {
	return nil
}

// SetWriteDeadline implements interface.
func (s *StdioStream) SetWriteDeadline(t time.Time) error {
	return nil
}

// StdinAddr is the struct for the stdi.
type StdinAddr struct {
	s string
}

// NewStdinAddr creates a new StdinAddr.
func NewStdinAddr(s string) *StdinAddr {
	return &StdinAddr{s}
}

// Network implements interface.
func (a *StdinAddr) Network() string {
	return "stdio"
}

func (a *StdinAddr) String() string {
	return a.s
}
