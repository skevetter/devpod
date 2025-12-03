package network

import (
	"context"
	"testing"

	"github.com/loft-sh/log"
	"github.com/stretchr/testify/suite"
)

type ServerTestSuite struct {
	suite.Suite
	server *Server
}

func TestServerTestSuite(t *testing.T) {
	suite.Run(t, new(ServerTestSuite))
}

func (s *ServerTestSuite) SetupTest() {
	logger := log.Default.ErrorStreamOnly()
	config := ServerConfig{
		Addr:           "localhost:0",
		GRPCTargetAddr: "localhost:50051",
		HTTPTargetAddr: "localhost:8080",
	}
	s.server = NewServer(config, logger)
}

func (s *ServerTestSuite) TearDownTest() {
	if s.server != nil {
		s.server.Stop()
	}
}

func (s *ServerTestSuite) TestNewServer() {
	logger := log.Default.ErrorStreamOnly()
	config := ServerConfig{Addr: "localhost:0"}
	server := NewServer(config, logger)
	s.NotNil(server)
	s.NotNil(server.Tracker())
	s.NotNil(server.Forwarder())
	s.NotNil(server.NetworkMap())
}

func (s *ServerTestSuite) TestTracker() {
	tracker := s.server.Tracker()
	s.NotNil(tracker)
	s.Equal(0, tracker.Count())
}

func (s *ServerTestSuite) TestForwarder() {
	forwarder := s.server.Forwarder()
	s.NotNil(forwarder)
	s.Equal(0, len(forwarder.List()))
}

func (s *ServerTestSuite) TestNetworkMap() {
	netmap := s.server.NetworkMap()
	s.NotNil(netmap)
	s.Equal(0, netmap.Count())
}

func (s *ServerTestSuite) TestStop() {
	err := s.server.Stop()
	s.NoError(err)
}

func (s *ServerTestSuite) TestStartRequiresValidAddr() {
	logger := log.Default.ErrorStreamOnly()
	config := ServerConfig{Addr: "invalid:99999"}
	server := NewServer(config, logger)

	ctx := context.Background()
	err := server.Start(ctx)
	s.Error(err)
}
