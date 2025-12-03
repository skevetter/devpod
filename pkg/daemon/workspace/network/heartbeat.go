package network

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/loft-sh/log"
	"tailscale.com/client/tailscale"
	"tailscale.com/tsnet"
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

// HeartbeatService sends periodic heartbeats when there are active connections.
type HeartbeatService struct {
	tsServer      *tsnet.Server
	lc            *tailscale.LocalClient
	config        *ServerConfig
	projectName   string
	workspaceName string
	log           log.Logger
	tracker       *ConnTracker
}

// NewHeartbeatService creates a new HeartbeatService.
func NewHeartbeatService(config *ServerConfig, tsServer *tsnet.Server, lc *tailscale.LocalClient, projectName, workspaceName string, tracker *ConnTracker, log log.Logger) *HeartbeatService {
	return &HeartbeatService{
		tsServer:      tsServer,
		lc:            lc,
		config:        config,
		projectName:   projectName,
		workspaceName: workspaceName,
		log:           log,
		tracker:       tracker,
	}
}

// Start begins the heartbeat loop.
func (s *HeartbeatService) Start(ctx context.Context) {
	s.log.Info("HeartbeatService: Start")
	transport := &http.Transport{DialContext: s.tsServer.Dial}
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.log.Info("HeartbeatService: Exit")
			return
		case <-ticker.C:
			s.log.Debugf("HeartbeatService: checking connection count")
			if s.tracker.Count("HeartbeatService") > 0 {
				if err := s.sendHeartbeat(ctx, client); err != nil {
					s.log.Errorf("HeartbeatService: failed to send heartbeat: %v", err)
				}
			} else {
				s.log.Debugf("HeartbeatService: No active connections, skipping heartbeat.")
			}
		}
	}
}

func (s *HeartbeatService) sendHeartbeat(ctx context.Context, client *http.Client) error {
	hbCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	discoveredRunner, err := discoverRunner(hbCtx, s.lc, s.log)
	if err != nil {
		s.log.Errorf("HeartbeatService: failed to discover runner: %v", err)
		return fmt.Errorf("failed to discover runner: %w", err)
	}

	heartbeatURL := fmt.Sprintf("http://%s.ts.loft/devpod/%s/%s/heartbeat", discoveredRunner, s.projectName, s.workspaceName)
	s.log.Infof("HeartbeatService: sending heartbeat to %s, active connections: %d", heartbeatURL, s.tracker.Count("HeartbeatService"))
	req, err := http.NewRequestWithContext(hbCtx, "GET", heartbeatURL, nil)
	if err != nil {
		s.log.Errorf("HeartbeatService: failed to create request for %s: %v", heartbeatURL, err)
		return fmt.Errorf("failed to create request for %s: %w", heartbeatURL, err)
	}
	req.Header.Set("Authorization", "Bearer "+s.config.AccessKey)
	resp, err := client.Do(req)
	if err != nil {
		s.log.Errorf("HeartbeatService: request to %s failed: %v", heartbeatURL, err)
		return fmt.Errorf("request to %s failed: %w", heartbeatURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		s.log.Errorf("HeartbeatService: received non-OK response from %s - Status: %d", heartbeatURL, resp.StatusCode)
		return fmt.Errorf("received response from %s - Status: %d", heartbeatURL, resp.StatusCode)
	}

	s.log.Debugf("HeartbeatService: received response from %s - Status: %d", heartbeatURL, resp.StatusCode)
	return nil
}
