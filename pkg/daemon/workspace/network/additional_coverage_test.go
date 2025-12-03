package network

import (
	"context"
	"testing"
	"time"

	"github.com/loft-sh/log"
	"github.com/stretchr/testify/suite"
)

type AdditionalCoverageTestSuite struct {
	suite.Suite
}

func TestAdditionalCoverageTestSuite(t *testing.T) {
	suite.Run(t, new(AdditionalCoverageTestSuite))
}

func (s *AdditionalCoverageTestSuite) TestMockConnDeadlines() {
	conn := &mockConn{}
	err := conn.SetReadDeadline(time.Now())
	s.NoError(err)
	err = conn.SetWriteDeadline(time.Now())
	s.NoError(err)
}

func (s *AdditionalCoverageTestSuite) TestParseHostPortErrors() {
	// Test with invalid port
	_, _, err := ParseHostPort("localhost:invalid")
	s.Error(err)

	// Test with no port
	_, _, err = ParseHostPort("localhost")
	s.Error(err)
}

func (s *AdditionalCoverageTestSuite) TestGetFreePortError() {
	// GetFreePort should always succeed
	port, err := GetFreePort()
	s.NoError(err)
	s.Greater(port, 0)
}

func (s *AdditionalCoverageTestSuite) TestHeartbeatDoubleStart() {
	tracker := NewConnectionTracker()
	config := HeartbeatConfig{
		Interval: 100 * time.Millisecond,
		Timeout:  200 * time.Millisecond,
	}
	hb := NewHeartbeat(config, tracker)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go hb.Start(ctx)
	time.Sleep(10 * time.Millisecond)

	// Second start should return immediately
	err := hb.Start(ctx)
	s.NoError(err)

	hb.Stop()
}

func (s *AdditionalCoverageTestSuite) TestNetworkMapUpdateNonExistent() {
	nm := NewNetworkMap()
	// Updating non-existent peer should not panic
	s.NotPanics(func() {
		nm.UpdatePeer("nonexistent")
	})
}

func (s *AdditionalCoverageTestSuite) TestConnectionTrackerUpdateNonExistent() {
	ct := NewConnectionTracker()
	// Updating non-existent connection should not panic
	s.NotPanics(func() {
		ct.Update("nonexistent")
	})
}

func (s *AdditionalCoverageTestSuite) TestClientSetTimeout() {
	client := NewClient("localhost:8080")
	client.SetTimeout(5 * time.Second)
	s.Equal(5*time.Second, client.timeout)
}

func (s *AdditionalCoverageTestSuite) TestServerStopWithoutStart() {
	logger := log.Default.ErrorStreamOnly()
	config := ServerConfig{Addr: "localhost:0"}
	server := NewServer(config, logger)

	// Stop without start should not panic
	err := server.Stop()
	s.NoError(err)
}

func (s *AdditionalCoverageTestSuite) TestGRPCProxyStopWithoutConn() {
	config := GRPCProxyConfig{TargetAddr: "localhost:50051"}
	proxy := NewGRPCProxy(config)
	err := proxy.Stop()
	s.NoError(err)
}
