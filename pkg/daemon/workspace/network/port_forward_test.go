package network

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/loft-sh/log"
	"github.com/stretchr/testify/suite"
)

type PortForwarderTestSuite struct {
	suite.Suite
	forwarder *PortForwarder
}

func TestPortForwarderTestSuite(t *testing.T) {
	suite.Run(t, new(PortForwarderTestSuite))
}

func (s *PortForwarderTestSuite) SetupTest() {
	logger := log.Default.ErrorStreamOnly()
	s.forwarder = NewPortForwarder(logger)
}

func (s *PortForwarderTestSuite) TearDownTest() {
	for _, port := range s.forwarder.List() {
		s.forwarder.Stop(port)
	}
}

func (s *PortForwarderTestSuite) TestNewPortForwarder() {
	logger := log.Default.ErrorStreamOnly()
	pf := NewPortForwarder(logger)
	s.NotNil(pf)
	s.Equal(0, len(pf.List()))
}

func (s *PortForwarderTestSuite) TestForward() {
	port, err := GetFreePort()
	s.NoError(err)

	ctx := context.Background()
	err = s.forwarder.Forward(ctx, fmt.Sprintf("%d", port), "localhost:8080")
	s.NoError(err)

	ports := s.forwarder.List()
	s.Equal(1, len(ports))
}

func (s *PortForwarderTestSuite) TestForwardDuplicate() {
	port, err := GetFreePort()
	s.NoError(err)
	portStr := fmt.Sprintf("%d", port)

	ctx := context.Background()
	err = s.forwarder.Forward(ctx, portStr, "localhost:8080")
	s.NoError(err)

	err = s.forwarder.Forward(ctx, portStr, "localhost:8080")
	s.Error(err)
}

func (s *PortForwarderTestSuite) TestStop() {
	port, err := GetFreePort()
	s.NoError(err)
	portStr := fmt.Sprintf("%d", port)

	ctx := context.Background()
	err = s.forwarder.Forward(ctx, portStr, "localhost:8080")
	s.NoError(err)

	err = s.forwarder.Stop(portStr)
	s.NoError(err)

	s.Equal(0, len(s.forwarder.List()))
}

func (s *PortForwarderTestSuite) TestStopNonExistent() {
	err := s.forwarder.Stop("99999")
	s.Error(err)
}

func (s *PortForwarderTestSuite) TestList() {
	s.Equal(0, len(s.forwarder.List()))

	port1, _ := GetFreePort()
	port2, _ := GetFreePort()

	ctx := context.Background()
	s.forwarder.Forward(ctx, fmt.Sprintf("%d", port1), "localhost:8080")
	s.forwarder.Forward(ctx, fmt.Sprintf("%d", port2), "localhost:8081")

	s.Equal(2, len(s.forwarder.List()))
}

func (s *PortForwarderTestSuite) TestForwardConnection() {
	// Create a test server
	listener, err := net.Listen("tcp", "localhost:0")
	s.NoError(err)
	defer listener.Close()

	serverAddr := listener.Addr().String()
	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			conn.Write([]byte("hello"))
			conn.Close()
		}
	}()

	// Forward to the test server
	localPort, err := GetFreePort()
	s.NoError(err)

	ctx := context.Background()
	err = s.forwarder.Forward(ctx, fmt.Sprintf("%d", localPort), serverAddr)
	s.NoError(err)

	// Give it time to start
	time.Sleep(50 * time.Millisecond)

	// Connect to forwarded port
	conn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", localPort))
	if err == nil {
		defer conn.Close()
		buf := make([]byte, 5)
		conn.Read(buf)
		s.Equal("hello", string(buf))
	}
}
