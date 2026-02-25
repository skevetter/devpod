package stdio

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
)

type StdioStreamSuite struct {
	suite.Suite
}

func TestStdioStreamSuite(t *testing.T) {
	suite.Run(t, new(StdioStreamSuite))
}

func (s *StdioStreamSuite) TestStdioStreamBuffering() {
	input := strings.NewReader("test input")
	output := &bytes.Buffer{}
	wc := &testWriteCloser{Buffer: output}

	stream := NewStdioStream(input, wc, false, 0)

	// Test write and flush
	testData := []byte("test escape sequence \x1b[2J")
	n, err := stream.Write(testData)
	s.NoError(err, "Write should not fail")
	s.Equal(len(testData), n, "Should write all bytes")

	// Verify data was flushed immediately
	s.Equal(testData, output.Bytes(), "Output should match input")
}

func (s *StdioStreamSuite) TestStdioStreamRead() {
	testData := "test data with escape sequences \x1b[H\x1b[2J"
	input := strings.NewReader(testData)
	output := &bytes.Buffer{}
	wc := &testWriteCloser{Buffer: output}

	stream := NewStdioStream(input, wc, false, 0)

	// Read data
	buf := make([]byte, 1024)
	n, err := stream.Read(buf)
	s.True(err == nil || err == io.EOF, "Read should succeed or return EOF")
	s.Equal(len(testData), n, "Should read all bytes")
	s.Equal(testData, string(buf[:n]), "Read data should match")
}

func (s *StdioStreamSuite) TestStdioStreamClose() {
	input := strings.NewReader("")
	output := &bytes.Buffer{}
	wc := &testWriteCloser{Buffer: output}

	stream := NewStdioStream(input, wc, false, 0)

	testData := []byte("buffered data")
	_, _ = stream.out.Write(testData) // Write to buffer without flushing

	err := stream.Close()
	s.NoError(err, "Close should not fail")
	s.True(wc.closed, "Underlying writer should be closed")
}

type testWriteCloser struct {
	*bytes.Buffer
	closed bool
}

func (w *testWriteCloser) Close() error {
	w.closed = true
	return nil
}
