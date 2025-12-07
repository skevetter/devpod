package network

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type HeartbeatTestSuite struct {
	suite.Suite
	tracker   *ConnectionTracker
	heartbeat *Heartbeat
}

func TestHeartbeatTestSuite(t *testing.T) {
	suite.Run(t, new(HeartbeatTestSuite))
}

func (s *HeartbeatTestSuite) SetupTest() {
	s.tracker = NewConnectionTracker()
	config := HeartbeatConfig{
		Interval: 50 * time.Millisecond,
		Timeout:  100 * time.Millisecond,
	}
	s.heartbeat = NewHeartbeat(config, s.tracker)
}

func (s *HeartbeatTestSuite) TearDownTest() {
	s.heartbeat.Stop()
}

func (s *HeartbeatTestSuite) TestNewHeartbeat() {
	config := DefaultHeartbeatConfig()
	hb := NewHeartbeat(config, s.tracker)
	s.NotNil(hb)
	s.False(hb.IsRunning())
}

func (s *HeartbeatTestSuite) TestStartStop() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = s.heartbeat.Start(ctx) }()
	time.Sleep(10 * time.Millisecond)
	s.True(s.heartbeat.IsRunning())

	s.heartbeat.Stop()
	time.Sleep(10 * time.Millisecond)
	s.False(s.heartbeat.IsRunning())
}

func (s *HeartbeatTestSuite) TestRemovesStaleConnections() {
	s.tracker.Add("conn1", "192.168.1.1:8080")
	s.Equal(1, s.tracker.Count())

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	go func() { _ = s.heartbeat.Start(ctx) }()
	time.Sleep(150 * time.Millisecond)

	s.Equal(0, s.tracker.Count())
}

func (s *HeartbeatTestSuite) TestKeepsActiveConnections() {
	s.tracker.Add("conn1", "192.168.1.1:8080")

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	go func() { _ = s.heartbeat.Start(ctx) }()

	// Update connection periodically
	ticker := time.NewTicker(40 * time.Millisecond)
	defer ticker.Stop()

	for i := 0; i < 4; i++ {
		<-ticker.C
		s.tracker.Update("conn1")
	}

	s.Equal(1, s.tracker.Count())
}
