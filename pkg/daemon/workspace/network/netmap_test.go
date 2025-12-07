package network

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type NetworkMapTestSuite struct {
	suite.Suite
	netmap *NetworkMap
}

func TestNetworkMapTestSuite(t *testing.T) {
	suite.Run(t, new(NetworkMapTestSuite))
}

func (s *NetworkMapTestSuite) SetupTest() {
	s.netmap = NewNetworkMap()
}

func (s *NetworkMapTestSuite) TestNewNetworkMap() {
	nm := NewNetworkMap()
	s.NotNil(nm)
	s.Equal(0, nm.Count())
}

func (s *NetworkMapTestSuite) TestAddPeer() {
	s.netmap.AddPeer("peer1", "192.168.1.1:8080")
	s.Equal(1, s.netmap.Count())

	peer, exists := s.netmap.GetPeer("peer1")
	s.True(exists)
	s.Equal("peer1", peer.ID)
	s.Equal("192.168.1.1:8080", peer.Addr)
}

func (s *NetworkMapTestSuite) TestRemovePeer() {
	s.netmap.AddPeer("peer1", "192.168.1.1:8080")
	s.Equal(1, s.netmap.Count())

	s.netmap.RemovePeer("peer1")
	s.Equal(0, s.netmap.Count())

	_, exists := s.netmap.GetPeer("peer1")
	s.False(exists)
}

func (s *NetworkMapTestSuite) TestGetPeer() {
	s.netmap.AddPeer("peer1", "192.168.1.1:8080")

	peer, exists := s.netmap.GetPeer("peer1")
	s.True(exists)
	s.NotNil(peer)

	_, exists = s.netmap.GetPeer("nonexistent")
	s.False(exists)
}

func (s *NetworkMapTestSuite) TestListPeers() {
	s.netmap.AddPeer("peer1", "192.168.1.1:8080")
	s.netmap.AddPeer("peer2", "192.168.1.2:8080")

	peers := s.netmap.ListPeers()
	s.Equal(2, len(peers))
}

func (s *NetworkMapTestSuite) TestUpdatePeer() {
	s.netmap.AddPeer("peer1", "192.168.1.1:8080")

	peer, _ := s.netmap.GetPeer("peer1")
	firstSeen := peer.LastSeen

	time.Sleep(10 * time.Millisecond)
	s.netmap.UpdatePeer("peer1")

	peer, _ = s.netmap.GetPeer("peer1")
	s.True(peer.LastSeen.After(firstSeen))
}

func (s *NetworkMapTestSuite) TestCount() {
	s.Equal(0, s.netmap.Count())

	s.netmap.AddPeer("peer1", "192.168.1.1:8080")
	s.Equal(1, s.netmap.Count())

	s.netmap.AddPeer("peer2", "192.168.1.2:8080")
	s.Equal(2, s.netmap.Count())

	s.netmap.RemovePeer("peer1")
	s.Equal(1, s.netmap.Count())
}
