package network

import (
	"sync/atomic"
	"time"
)

type Metrics struct {
	ActiveConnections int64
	TotalRequests     int64
	FailedRequests    int64
	TotalLatencyNs    int64
}

func (m *Metrics) RecordRequest(success bool, latency time.Duration) {
	atomic.AddInt64(&m.TotalRequests, 1)
	if !success {
		atomic.AddInt64(&m.FailedRequests, 1)
	}
	atomic.AddInt64(&m.TotalLatencyNs, int64(latency))
}

func (m *Metrics) IncrementActive() {
	atomic.AddInt64(&m.ActiveConnections, 1)
}

func (m *Metrics) DecrementActive() {
	atomic.AddInt64(&m.ActiveConnections, -1)
}

func (m *Metrics) AvgLatency() time.Duration {
	total := atomic.LoadInt64(&m.TotalRequests)
	if total == 0 {
		return 0
	}
	return time.Duration(atomic.LoadInt64(&m.TotalLatencyNs) / total)
}
