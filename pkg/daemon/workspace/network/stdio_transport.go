package network

import (
	"context"
	"io"
	"net"
	"time"
)

type StdioTransport struct {
	stdin  io.Reader
	stdout io.Writer
}

func NewStdioTransport(stdin io.Reader, stdout io.Writer) *StdioTransport {
	return &StdioTransport{
		stdin:  stdin,
		stdout: stdout,
	}
}

func (s *StdioTransport) Dial(ctx context.Context, target string) (Conn, error) {
	return &stdioConn{
		reader: s.stdin,
		writer: s.stdout,
	}, nil
}

func (s *StdioTransport) Close() error {
	return nil
}

// stdioConn wraps stdin/stdout as a net.Conn
type stdioConn struct {
	reader io.Reader
	writer io.Writer
}

func (s *stdioConn) Read(b []byte) (n int, err error)   { return s.reader.Read(b) }
func (s *stdioConn) Write(b []byte) (n int, err error)  { return s.writer.Write(b) }
func (s *stdioConn) Close() error                       { return nil }
func (s *stdioConn) LocalAddr() net.Addr                { return nil }
func (s *stdioConn) RemoteAddr() net.Addr               { return nil }
func (s *stdioConn) SetDeadline(t time.Time) error      { return nil }
func (s *stdioConn) SetReadDeadline(t time.Time) error  { return nil }
func (s *stdioConn) SetWriteDeadline(t time.Time) error { return nil }
