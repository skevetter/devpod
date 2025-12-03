package local

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/loft-sh/log"
	"github.com/mwitkow/grpc-proxy/proxy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// GRPCProxyServer is a client-side gRPC proxy
type GRPCProxyServer struct {
	targetAddr string
	listener   net.Listener
	server     *grpc.Server
	conn       *grpc.ClientConn
	log        log.Logger
	mu         sync.Mutex
}

// NewGRPCProxyServer creates a new client-side gRPC proxy
func NewGRPCProxyServer(targetAddr string, log log.Logger) *GRPCProxyServer {
	return &GRPCProxyServer{
		targetAddr: targetAddr,
		log:        log,
	}
}

// Start starts the gRPC proxy server
func (s *GRPCProxyServer) Start(ctx context.Context, listener net.Listener) error {
	s.listener = listener

	// Create director
	director := func(ctx context.Context, fullMethodName string) (context.Context, *grpc.ClientConn, error) {
		if s.conn == nil {
			conn, err := grpc.NewClient(s.targetAddr,
				grpc.WithTransportCredentials(insecure.NewCredentials()),
			)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to dial target: %w", err)
			}
			s.conn = conn
		}
		return ctx, s.conn, nil
	}

	// Create server
	s.mu.Lock()
	s.server = grpc.NewServer(
		grpc.UnknownServiceHandler(proxy.TransparentHandler(director)),
	)
	server := s.server
	s.mu.Unlock()

	s.log.Infof("Starting gRPC proxy on %s", listener.Addr())
	return server.Serve(listener)
}

// Stop stops the gRPC proxy server
func (s *GRPCProxyServer) Stop() error {
	s.mu.Lock()
	server := s.server
	conn := s.conn
	s.mu.Unlock()

	if server != nil {
		server.GracefulStop()
	}
	if conn != nil {
		return conn.Close()
	}
	return nil
}
