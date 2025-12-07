package network

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
)

type GRPCProxyTestSuite struct {
	suite.Suite
	proxy *GRPCProxy
}

func TestGRPCProxyTestSuite(t *testing.T) {
	suite.Run(t, new(GRPCProxyTestSuite))
}

func (s *GRPCProxyTestSuite) SetupTest() {
	config := GRPCProxyConfig{
		TargetAddr: "localhost:50051",
	}
	s.proxy = NewGRPCProxy(config)
}

func (s *GRPCProxyTestSuite) TearDownTest() {
	if s.proxy != nil {
		_ = s.proxy.Stop()
	}
}

func (s *GRPCProxyTestSuite) TestNewGRPCProxy() {
	config := GRPCProxyConfig{TargetAddr: "localhost:50051"}
	proxy := NewGRPCProxy(config)
	s.NotNil(proxy)
	s.Equal("localhost:50051", proxy.config.TargetAddr)
}

func (s *GRPCProxyTestSuite) TestStart() {
	err := s.proxy.Start(context.Background())
	s.NoError(err)
	s.NotNil(s.proxy.Server())
}

func (s *GRPCProxyTestSuite) TestStop() {
	err := s.proxy.Start(context.Background())
	s.NoError(err)

	err = s.proxy.Stop()
	s.NoError(err)
}
