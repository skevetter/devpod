package network_test

import (
	"context"
	"errors"
	"testing"

	"github.com/skevetter/devpod/pkg/daemon/workspace/network"
	"github.com/stretchr/testify/suite"
)

type HealthTestSuite struct {
	suite.Suite
}

func (s *HealthTestSuite) TestHealthCheckSuccess() {
	mock := &network.MockTransport{
		DialFunc: func(ctx context.Context, target string) (network.Conn, error) {
			return nil, nil
		},
	}

	status := network.CheckHealth(context.Background(), mock)
	s.True(status.Healthy)
	s.Equal("mock", status.Transport)
}

func (s *HealthTestSuite) TestHealthCheckFailure() {
	mock := &network.MockTransport{
		DialFunc: func(ctx context.Context, target string) (network.Conn, error) {
			return nil, errors.New("connection failed")
		},
	}

	status := network.CheckHealth(context.Background(), mock)
	s.False(status.Healthy)
	s.Equal("connection failed", status.Error)
}

func TestHealthTestSuite(t *testing.T) {
	suite.Run(t, new(HealthTestSuite))
}
