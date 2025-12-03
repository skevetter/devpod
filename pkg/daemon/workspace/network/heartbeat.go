package network

import (
	"context"
	"sync"
	"time"
)

// HeartbeatConfig configures the heartbeat system
type HeartbeatConfig struct {
	Interval time.Duration
	Timeout  time.Duration
}

// DefaultHeartbeatConfig returns default configuration
func DefaultHeartbeatConfig() HeartbeatConfig {
	return HeartbeatConfig{
		Interval: 30 * time.Second,
		Timeout:  90 * time.Second,
	}
}

// Heartbeat monitors connection health
type Heartbeat struct {
	config  HeartbeatConfig
	tracker *ConnectionTracker
	mu      sync.RWMutex
	running bool
	cancel  context.CancelFunc
}

// NewHeartbeat creates a new heartbeat monitor
func NewHeartbeat(config HeartbeatConfig, tracker *ConnectionTracker) *Heartbeat {
	return &Heartbeat{
		config:  config,
		tracker: tracker,
	}
}

// Start starts the heartbeat monitor
func (h *Heartbeat) Start(ctx context.Context) error {
	h.mu.Lock()
	if h.running {
		h.mu.Unlock()
		return nil
	}
	h.running = true
	ctx, cancel := context.WithCancel(ctx)
	h.cancel = cancel
	h.mu.Unlock()

	ticker := time.NewTicker(h.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			h.checkConnections()
		}
	}
}

// Stop stops the heartbeat monitor
func (h *Heartbeat) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.running && h.cancel != nil {
		h.cancel()
		h.running = false
	}
}

// IsRunning returns whether the heartbeat is running
func (h *Heartbeat) IsRunning() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.running
}

// checkConnections checks for stale connections
func (h *Heartbeat) checkConnections() {
	conns := h.tracker.List()
	now := time.Now()

	for _, conn := range conns {
		if now.Sub(conn.LastSeen) > h.config.Timeout {
			h.tracker.Remove(conn.ID)
		}
	}
}
