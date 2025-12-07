package network

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/loft-sh/log"
	"github.com/skevetter/devpod/pkg/platform/client"
	"github.com/skevetter/devpod/pkg/ts"
	"tailscale.com/tsnet"
)

type ServerConfig struct {
	AccessKey     string
	PlatformHost  string
	WorkspaceHost string
	LogF          func(format string, args ...any)
	Client        client.Client
	RootDir       string
}

// Server is the main workspace network server with integrated observability
type Server struct {
	network     *tsnet.Server
	config      *ServerConfig
	log         log.Logger
	connTracker *ConnTracker
	metrics     *Metrics

	// Services
	sshSvc        *SSHService
	httpProxySvc  *HTTPPortForwardService
	netProxySvc   *NetworkProxyService
	heartbeatSvc  *HeartbeatService
	netmapWatcher *NetmapWatcherService
}

// NewServer creates a new Server with observability
func NewServer(config *ServerConfig, logger log.Logger) *Server {
	return &Server{
		config: config,
		log:    logger,
		connTracker: &ConnTracker{
			logger: logger,
		},
		metrics: &Metrics{},
	}
}

// Start initializes the network server and all services with observability
func (s *Server) Start(ctx context.Context) error {
	s.log.Infof("Starting workspace server")
	s.metrics.IncrementActive()
	defer s.metrics.DecrementActive()

	workspaceName, projectName, err := s.joinNetwork(ctx)
	if err != nil {
		s.metrics.RecordRequest(false, 0)
		return err
	}
	s.metrics.RecordRequest(true, 0)

	lc, err := s.network.LocalClient()
	if err != nil {
		return err
	}

	// Create and start the SSH service
	s.sshSvc, err = NewSSHService(s.network, s.connTracker, s.log)
	if err != nil {
		return err
	}
	s.sshSvc.Start(ctx)

	// Create and start HTTP port forward service
	s.httpProxySvc, err = NewHTTPPortForwardService(s.network, s.connTracker, s.log)
	if err != nil {
		return err
	}
	s.httpProxySvc.Start(ctx)

	// Create and start network proxy service (includes git-credentials)
	networkSocket := filepath.Join(s.config.RootDir, NetworkProxySocket)
	s.netProxySvc, err = NewNetworkProxyService(networkSocket, s.network, lc, s.config, projectName, workspaceName, s.log)
	if err != nil {
		return err
	}
	go func() { _ = s.netProxySvc.Start(ctx) }()

	// Start heartbeat service
	s.heartbeatSvc = NewHeartbeatService(s.config, s.network, lc, projectName, workspaceName, s.connTracker, s.log)
	go s.heartbeatSvc.Start(ctx)

	// Start netmap watcher
	s.netmapWatcher = NewNetmapWatcherService(s.config.RootDir, lc, s.log)
	go s.netmapWatcher.Start(ctx)

	s.log.Infof("All services started successfully")

	// Wait until context is canceled
	<-ctx.Done()
	return nil
}

// Metrics returns the server metrics for observability
func (s *Server) Metrics() *Metrics {
	return s.metrics
}

// HealthStatus represents the health status of the server
type HealthStatus struct {
	Healthy   bool
	Transport string
	Error     string
}

// Health returns health status of the workspace server
func (s *Server) Health() HealthStatus {
	return HealthStatus{
		Healthy:   s.network != nil,
		Transport: "tailscale",
		Error:     "",
	}
}

// Stop shuts down all services and the network server.
func (s *Server) Stop() {
	if s.sshSvc != nil {
		s.sshSvc.Stop()
	}
	if s.httpProxySvc != nil {
		s.httpProxySvc.Stop()
	}
	if s.netProxySvc != nil {
		s.netProxySvc.Stop()
	}
	if s.network != nil {
		_ = s.network.Close()
		s.network = nil
	}
	s.log.Info("Workspace server stopped")
}

// Dial dials the given address using the network server.
func (s *Server) Dial(ctx context.Context, network, addr string) (net.Conn, error) {
	if s.network == nil {
		return nil, fmt.Errorf("network server is not running")
	}
	return s.network.Dial(ctx, network, addr)
}

// joinNetwork validates configuration, sets up the control URL, starts the network server,
// and parses the hostname into workspace and project names.
func (s *Server) joinNetwork(ctx context.Context) (workspace, project string, err error) {
	if err = s.validateConfig(); err != nil {
		return "", "", err
	}
	baseURL, err := s.setupControlURL(ctx)
	if err != nil {
		return "", "", err
	}
	if err = s.initNetworkServer(ctx, baseURL); err != nil {
		return "", "", err
	}
	return s.parseWorkspaceHostname()
}

func (s *Server) validateConfig() error {
	if s.config.AccessKey == "" || s.config.PlatformHost == "" || s.config.WorkspaceHost == "" {
		return fmt.Errorf("access key, host, or hostname cannot be empty")
	}
	return nil
}

func (s *Server) setupControlURL(ctx context.Context) (*url.URL, error) {
	baseURL := &url.URL{
		Scheme: ts.GetEnvOrDefault("LOFT_TSNET_SCHEME", "https"),
		Host:   s.config.PlatformHost,
	}
	if err := ts.CheckDerpConnection(ctx, baseURL); err != nil {
		return nil, fmt.Errorf("failed to verify DERP connection: %w", err)
	}
	return baseURL, nil
}

func (s *Server) initNetworkServer(ctx context.Context, controlURL *url.URL) error {
	s.log.Infof("connecting to control URL %s/coordinator/", controlURL.String())
	var err error
	s.network, err = ts.StartServer(ctx, &ts.ServerConfig{
		Hostname:   s.config.WorkspaceHost,
		AuthKey:    s.config.AccessKey,
		Dir:        s.config.RootDir,
		ControlURL: controlURL,
		Ephemeral:  true,
		Logf:       s.config.LogF,
	})
	return err
}

func (s *Server) parseWorkspaceHostname() (workspace, project string, err error) {
	parts := strings.Split(s.config.WorkspaceHost, ".")
	if len(parts) < 4 {
		return "", "", fmt.Errorf("invalid workspace hostname format: %s", s.config.WorkspaceHost)
	}
	return parts[1], parts[2], nil
}
