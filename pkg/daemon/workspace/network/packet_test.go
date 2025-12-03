package network_test

import (
	"testing"

	"github.com/skevetter/devpod/pkg/daemon/workspace/network"
	"github.com/stretchr/testify/suite"
)

type PacketTestSuite struct {
	suite.Suite
}

func (s *PacketTestSuite) TestTypicalCredentialSize() {
	// Test with various credential payloads
	gitCred := []byte("username:password")
	dockerCred := []byte(`{"username":"user","password":"pass"}`)

	s.Less(len(gitCred), 4096, "Git credentials should fit in single packet")
	s.Less(len(dockerCred), 4096, "Docker credentials should fit in single packet")
}

func (s *PacketTestSuite) TestOptimalPacketSize() {
	optimizer := network.NewPacketOptimizer()
	s.Equal(4096, optimizer.BufferSize())
}

func TestPacketTestSuite(t *testing.T) {
	suite.Run(t, new(PacketTestSuite))
}
