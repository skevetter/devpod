package network

import (
	"context"
	"time"
)

type HealthStatus struct {
	Healthy   bool
	Transport string
	Latency   time.Duration
	Error     string
}

func CheckHealth(ctx context.Context, transport Transport) HealthStatus {
	start := time.Now()
	conn, err := transport.Dial(ctx, "")
	latency := time.Since(start)

	transportType := "unknown"
	switch transport.(type) {
	case *HTTPTransport:
		transportType = "http"
	case *StdioTransport:
		transportType = "stdio"
	case *FallbackTransport:
		transportType = "fallback"
	case *MockTransport:
		transportType = "mock"
	}

	if err != nil {
		return HealthStatus{
			Healthy:   false,
			Transport: transportType,
			Latency:   latency,
			Error:     err.Error(),
		}
	}

	if conn != nil {
		_ = conn.Close()
	}

	return HealthStatus{
		Healthy:   true,
		Transport: transportType,
		Latency:   latency,
	}
}
