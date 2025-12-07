package network

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/loft-sh/log"
	"tailscale.com/client/tailscale"
	"tailscale.com/tsnet"
)

const hbLogPrefix = "HeartbeatService: "

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
	s.log.Info(hbLogPrefix + "start")
	transport := &http.Transport{DialContext: s.tsServer.Dial}
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.log.Info(hbLogPrefix + "Exit")
			return
		case <-ticker.C:
			s.log.Debugf(hbLogPrefix + "checking connection count")
			if s.tracker.Count("HeartbeatService") > 0 {
				if err := s.sendHeartbeat(ctx, client); err != nil {
					s.log.Errorf(hbLogPrefix+"failed to send heartbeat %v", err)
				}
			} else {
				s.log.Debugf(hbLogPrefix + "no active connections, skipping heartbeat")
			}
		}
	}
}

func (s *HeartbeatService) sendHeartbeat(ctx context.Context, client *http.Client) error {
	hbCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	discoveredRunner, err := discoverRunner(hbCtx, s.lc, s.log)
	if err != nil {
		s.log.Errorf(hbLogPrefix+"failed to discover runner %v", err)
		return fmt.Errorf("failed to discover runner %w", err)
	}

	heartbeatURL := fmt.Sprintf("http://%s.ts.loft/devpod/%s/%s/heartbeat", discoveredRunner, s.projectName, s.workspaceName)
	s.log.Infof(hbLogPrefix+"sending heartbeat to %s, active connections %d", heartbeatURL, s.tracker.Count("HeartbeatService"))
	req, err := http.NewRequestWithContext(hbCtx, "GET", heartbeatURL, nil)
	if err != nil {
		s.log.Errorf(hbLogPrefix+"failed to create request for %s %v", heartbeatURL, err)
		return fmt.Errorf("failed to create request for %s %w", heartbeatURL, err)
	}
	req.Header.Set("Authorization", "Bearer "+s.config.AccessKey)
	resp, err := client.Do(req)
	if err != nil {
		s.log.Errorf(hbLogPrefix+"request to %s failed %v", heartbeatURL, err)
		return fmt.Errorf("request to %s failed %w", heartbeatURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		s.log.Errorf(hbLogPrefix+"received non-OK response from %s with status %d", heartbeatURL, resp.StatusCode)
		return fmt.Errorf("received response from %s with status %d", heartbeatURL, resp.StatusCode)
	}

	s.log.Debugf(hbLogPrefix+"received response from %s with status %d", heartbeatURL, resp.StatusCode)
	return nil
}
