package network

import (
	"context"
	"fmt"
	"net"

	"github.com/loft-sh/log"
	"github.com/soheilhy/cmux"
)

// ServerConfig configures the network server
type ServerConfig struct {
	Addr           string
	GRPCTargetAddr string
	HTTPTargetAddr string
}

// Server coordinates network services
type Server struct {
	config    ServerConfig
	listener  net.Listener
	mux       cmux.CMux
	grpcProxy *GRPCProxy
	httpProxy *HTTPProxyHandler
	tracker   *ConnectionTracker
	heartbeat *Heartbeat
	forwarder *PortForwarder
	netmap    *NetworkMap
	log       log.Logger
}

// NewServer creates a new network server
func NewServer(config ServerConfig, log log.Logger) *Server {
	tracker := NewConnectionTracker()
	return &Server{
		config:    config,
		tracker:   tracker,
		heartbeat: NewHeartbeat(DefaultHeartbeatConfig(), tracker),
		forwarder: NewPortForwarder(log),
		netmap:    NewNetworkMap(),
		log:       log,
	}
}

// Start starts the network server
func (s *Server) Start(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.config.Addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	s.listener = listener

	// Create multiplexer
	s.mux = cmux.New(listener)

	// Match gRPC and HTTP
	grpcListener := s.mux.Match(cmux.HTTP2HeaderField("content-type", "application/grpc"))
	httpListener := s.mux.Match(cmux.HTTP1Fast())

	// Start gRPC proxy
	if s.config.GRPCTargetAddr != "" {
		s.grpcProxy = NewGRPCProxy(GRPCProxyConfig{TargetAddr: s.config.GRPCTargetAddr})
		if err := s.grpcProxy.Start(ctx); err != nil {
			return fmt.Errorf("failed to start gRPC proxy: %w", err)
		}
		go s.grpcProxy.Server().Serve(grpcListener)
	}

	// Start HTTP proxy (simplified for now)
	if s.config.HTTPTargetAddr != "" {
		s.httpProxy = NewHTTPProxyHandler(s.config.HTTPTargetAddr)
		go httpListener.Close() // Close unused listener
	}

	// Start heartbeat
	go s.heartbeat.Start(ctx)

	s.log.Infof("Network server listening on %s", s.config.Addr)
	return s.mux.Serve()
}

// Stop stops the network server
func (s *Server) Stop() error {
	s.heartbeat.Stop()
	if s.grpcProxy != nil {
		s.grpcProxy.Stop()
	}
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

// Tracker returns the connection tracker
func (s *Server) Tracker() *ConnectionTracker {
	return s.tracker
}

// Forwarder returns the port forwarder
func (s *Server) Forwarder() *PortForwarder {
	return s.forwarder
}

// NetworkMap returns the network map
func (s *Server) NetworkMap() *NetworkMap {
	return s.netmap
}
