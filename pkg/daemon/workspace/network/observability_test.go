package network_test

import (
	"testing"
	"time"

	"github.com/skevetter/devpod/pkg/daemon/workspace/network"
	"github.com/stretchr/testify/suite"
)

type ObservabilityTestSuite struct {
	suite.Suite
}

func (s *ObservabilityTestSuite) TestMetricsRecordRequest() {
	m := &network.Metrics{}
	m.RecordRequest(true, 100*time.Millisecond)
	m.RecordRequest(false, 200*time.Millisecond)

	s.Equal(int64(2), m.TotalRequests)
	s.Equal(int64(1), m.FailedRequests)
}

func (s *ObservabilityTestSuite) TestMetricsActiveConnections() {
	m := &network.Metrics{}
	m.IncrementActive()
	m.IncrementActive()
	s.Equal(int64(2), m.ActiveConnections)
	m.DecrementActive()
	s.Equal(int64(1), m.ActiveConnections)
}

func TestObservabilityTestSuite(t *testing.T) {
	suite.Run(t, new(ObservabilityTestSuite))
}
