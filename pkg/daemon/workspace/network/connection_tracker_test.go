package network

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type ConnectionTrackerTestSuite struct {
	suite.Suite
	tracker *ConnectionTracker
}

func TestConnectionTrackerTestSuite(t *testing.T) {
	suite.Run(t, new(ConnectionTrackerTestSuite))
}

func (s *ConnectionTrackerTestSuite) SetupTest() {
	s.tracker = NewConnectionTracker()
}

func (s *ConnectionTrackerTestSuite) TestNewConnectionTracker() {
	tracker := NewConnectionTracker()
	s.NotNil(tracker)
	s.Equal(0, tracker.Count())
}

func (s *ConnectionTrackerTestSuite) TestAddConnection() {
	s.tracker.Add("conn1", "192.168.1.1:8080")
	s.Equal(1, s.tracker.Count())

	conn, exists := s.tracker.Get("conn1")
	s.True(exists)
	s.Equal("conn1", conn.ID)
	s.Equal("192.168.1.1:8080", conn.RemoteAddr)
}

func (s *ConnectionTrackerTestSuite) TestRemoveConnection() {
	s.tracker.Add("conn1", "192.168.1.1:8080")
	s.Equal(1, s.tracker.Count())

	s.tracker.Remove("conn1")
	s.Equal(0, s.tracker.Count())

	_, exists := s.tracker.Get("conn1")
	s.False(exists)
}

func (s *ConnectionTrackerTestSuite) TestUpdateConnection() {
	s.tracker.Add("conn1", "192.168.1.1:8080")

	conn, _ := s.tracker.Get("conn1")
	firstSeen := conn.LastSeen

	time.Sleep(10 * time.Millisecond)
	s.tracker.Update("conn1")

	conn, _ = s.tracker.Get("conn1")
	s.True(conn.LastSeen.After(firstSeen))
}

func (s *ConnectionTrackerTestSuite) TestListConnections() {
	s.tracker.Add("conn1", "192.168.1.1:8080")
	s.tracker.Add("conn2", "192.168.1.2:8080")

	conns := s.tracker.List()
	s.Equal(2, len(conns))
}

func (s *ConnectionTrackerTestSuite) TestConcurrentAccess() {
	done := make(chan bool)

	// Add connections concurrently
	for i := 0; i < 10; i++ {
		go func(id int) {
			s.tracker.Add(string(rune(id)), "192.168.1.1:8080")
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	s.Equal(10, s.tracker.Count())
}
