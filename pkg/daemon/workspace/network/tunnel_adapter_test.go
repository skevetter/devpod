package network_test

import (
	"context"
	"testing"

	"github.com/skevetter/devpod/pkg/agent/tunnel"
	"github.com/skevetter/devpod/pkg/daemon/workspace/network"
	"github.com/stretchr/testify/suite"
)

type TunnelAdapterTestSuite struct {
	suite.Suite
	client *network.TransportTunnelClient
}

func (s *TunnelAdapterTestSuite) SetupTest() {
	transport := network.NewStdioTransport(nil, nil)
	s.client = network.NewTransportTunnelClient(transport)
}

func (s *TunnelAdapterTestSuite) TestNewTransportTunnelClient() {
	transport := network.NewStdioTransport(nil, nil)
	client := network.NewTransportTunnelClient(transport)
	s.NotNil(client)
}

func (s *TunnelAdapterTestSuite) TestPing() {
	resp, err := s.client.Ping(context.Background(), &tunnel.Empty{})
	s.NoError(err)
	s.NotNil(resp)
}

func (s *TunnelAdapterTestSuite) TestForwardPort() {
	req := &tunnel.ForwardPortRequest{Port: "8080"}
	_, err := s.client.ForwardPort(context.Background(), req)
	s.NoError(err)
}

func (s *TunnelAdapterTestSuite) TestStopForwardPort() {
	req := &tunnel.StopForwardPortRequest{Port: "8080"}
	_, err := s.client.StopForwardPort(context.Background(), req)
	s.NoError(err)
}

func (s *TunnelAdapterTestSuite) TestLog() {
	msg := &tunnel.LogMessage{Message: "test"}
	_, err := s.client.Log(context.Background(), msg)
	s.NoError(err)
}

func (s *TunnelAdapterTestSuite) TestSendResult() {
	msg := &tunnel.Message{Message: "test"}
	_, err := s.client.SendResult(context.Background(), msg)
	s.NoError(err)
}

func TestTunnelAdapterTestSuite(t *testing.T) {
	suite.Run(t, new(TunnelAdapterTestSuite))
}
