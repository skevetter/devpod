package network

import (
	"context"
	"fmt"

	"github.com/mwitkow/grpc-proxy/proxy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// GRPCProxyConfig configures the gRPC proxy
type GRPCProxyConfig struct {
	TargetAddr string
}

// GRPCProxy is a gRPC reverse proxy
type GRPCProxy struct {
	config GRPCProxyConfig
	server *grpc.Server
	conn   *grpc.ClientConn
}

// NewGRPCProxy creates a new gRPC proxy
func NewGRPCProxy(config GRPCProxyConfig) *GRPCProxy {
	return &GRPCProxy{
		config: config,
	}
}

// Start starts the gRPC proxy
func (p *GRPCProxy) Start(ctx context.Context) error {
	// Create director function
	director := func(ctx context.Context, fullMethodName string) (context.Context, *grpc.ClientConn, error) {
		if p.conn == nil {
			conn, err := grpc.NewClient(p.config.TargetAddr,
				grpc.WithTransportCredentials(insecure.NewCredentials()),
			)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to dial target: %w", err)
			}
			p.conn = conn
		}
		return ctx, p.conn, nil
	}

	// Create proxy server
	p.server = grpc.NewServer(
		grpc.UnknownServiceHandler(proxy.TransparentHandler(director)),
	)

	return nil
}

// Stop stops the gRPC proxy
func (p *GRPCProxy) Stop() error {
	if p.server != nil {
		p.server.GracefulStop()
	}
	if p.conn != nil {
		return p.conn.Close()
	}
	return nil
}

// Server returns the gRPC server
func (p *GRPCProxy) Server() *grpc.Server {
	return p.server
}
