package network

import (
	"context"
	"fmt"
	"net"

	"github.com/loft-sh/log"
	"github.com/mwitkow/grpc-proxy/proxy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"tailscale.com/tsnet"
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

// GrpcDirector handles gRPC proxying with Tailscale
type GrpcDirector struct {
	tsServer *tsnet.Server
	log      log.Logger
}

// NewGrpcDirector creates a new gRPC director
func NewGrpcDirector(tsServer *tsnet.Server, log log.Logger) *GrpcDirector {
	return &GrpcDirector{
		tsServer: tsServer,
		log:      log,
	}
}

// DirectorFunc returns the director function for gRPC proxy
func (d *GrpcDirector) DirectorFunc(ctx context.Context, fullMethodName string) (context.Context, *grpc.ClientConn, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		d.log.Warnf("NetworkProxyService: gRPC: Director missing incoming metadata for call %q", fullMethodName)
		return nil, nil, fmt.Errorf("missing metadata")
	}
	mdCopy := md.Copy()

	targetHost := getHeader(mdCopy, HeaderTargetHost)
	targetPort := getHeader(mdCopy, HeaderTargetPort)
	proxyPort := getHeader(mdCopy, HeaderProxyPort)

	if targetHost == "" || targetPort == "" || proxyPort == "" {
		d.log.Errorf("NetworkProxyService: gRPC: Director missing x-target-host, x-proxy-port or x-target-port metadata for call %q", fullMethodName)
		return nil, nil, fmt.Errorf("missing x-target-host, x-proxy-port or x-target-port metadata")
	}

	target := fmt.Sprintf("%s:%s", targetHost, proxyPort)
	d.log.Debugf("NetworkProxyService: gRPC: Proxying call %q to target %s", fullMethodName, target)

	conn, err := grpc.NewClient(target,
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			return d.tsServer.Dial(ctx, "tcp", addr)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		d.log.Errorf("NetworkProxyService: gRPC: Failed to dial backend %s: %v", target, err)
		return nil, nil, fmt.Errorf("failed to dial backend: %w", err)
	}

	outCtx := metadata.NewOutgoingContext(ctx, mdCopy)
	return outCtx, conn, nil
}

func getHeader(md metadata.MD, key string) string {
	vals := md.Get(key)
	if len(vals) > 0 {
		return vals[0]
	}
	return ""
}
