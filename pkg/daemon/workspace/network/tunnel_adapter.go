package network

import (
	"context"
	"encoding/json"

	"github.com/skevetter/devpod/pkg/agent/tunnel"
	"google.golang.org/grpc"
)

// TransportTunnelClient adapts network.Transport to tunnel.TunnelClient interface
type TransportTunnelClient struct {
	transport Transport
}

func NewTransportTunnelClient(transport Transport) *TransportTunnelClient {
	return &TransportTunnelClient{transport: transport}
}

func (t *TransportTunnelClient) Ping(ctx context.Context, req *tunnel.Empty, opts ...grpc.CallOption) (*tunnel.Empty, error) {
	return &tunnel.Empty{}, nil
}

func (t *TransportTunnelClient) Log(ctx context.Context, msg *tunnel.LogMessage, opts ...grpc.CallOption) (*tunnel.Empty, error) {
	return &tunnel.Empty{}, nil
}

func (t *TransportTunnelClient) SendResult(ctx context.Context, msg *tunnel.Message, opts ...grpc.CallOption) (*tunnel.Empty, error) {
	return &tunnel.Empty{}, nil
}

func (t *TransportTunnelClient) sendMessage(ctx context.Context, msg *tunnel.Message) (*tunnel.Message, error) {
	conn, err := t.transport.Dial(ctx, "")
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()

	if err := json.NewEncoder(conn).Encode(msg); err != nil {
		return nil, err
	}

	var response tunnel.Message
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		return nil, err
	}

	return &response, nil
}

func (t *TransportTunnelClient) DockerCredentials(ctx context.Context, req *tunnel.Message, opts ...grpc.CallOption) (*tunnel.Message, error) {
	return t.sendMessage(ctx, req)
}

func (t *TransportTunnelClient) GitCredentials(ctx context.Context, req *tunnel.Message, opts ...grpc.CallOption) (*tunnel.Message, error) {
	return t.sendMessage(ctx, req)
}

func (t *TransportTunnelClient) GitSSHSignature(ctx context.Context, req *tunnel.Message, opts ...grpc.CallOption) (*tunnel.Message, error) {
	return t.sendMessage(ctx, req)
}

func (t *TransportTunnelClient) GitUser(ctx context.Context, req *tunnel.Empty, opts ...grpc.CallOption) (*tunnel.Message, error) {
	return t.sendMessage(ctx, &tunnel.Message{Message: "git-user"})
}

func (t *TransportTunnelClient) LoftConfig(ctx context.Context, req *tunnel.Message, opts ...grpc.CallOption) (*tunnel.Message, error) {
	return t.sendMessage(ctx, req)
}

func (t *TransportTunnelClient) GPGPublicKeys(ctx context.Context, req *tunnel.Message, opts ...grpc.CallOption) (*tunnel.Message, error) {
	return t.sendMessage(ctx, req)
}

func (t *TransportTunnelClient) KubeConfig(ctx context.Context, req *tunnel.Message, opts ...grpc.CallOption) (*tunnel.Message, error) {
	return t.sendMessage(ctx, req)
}

func (t *TransportTunnelClient) ForwardPort(ctx context.Context, req *tunnel.ForwardPortRequest, opts ...grpc.CallOption) (*tunnel.ForwardPortResponse, error) {
	return &tunnel.ForwardPortResponse{}, nil
}

func (t *TransportTunnelClient) StopForwardPort(ctx context.Context, req *tunnel.StopForwardPortRequest, opts ...grpc.CallOption) (*tunnel.StopForwardPortResponse, error) {
	return &tunnel.StopForwardPortResponse{}, nil
}

func (t *TransportTunnelClient) StreamGitClone(ctx context.Context, req *tunnel.Empty, opts ...grpc.CallOption) (grpc.ServerStreamingClient[tunnel.Chunk], error) {
	return nil, nil
}

func (t *TransportTunnelClient) StreamWorkspace(ctx context.Context, req *tunnel.Empty, opts ...grpc.CallOption) (grpc.ServerStreamingClient[tunnel.Chunk], error) {
	return nil, nil
}

func (t *TransportTunnelClient) StreamMount(ctx context.Context, req *tunnel.StreamMountRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[tunnel.Chunk], error) {
	return nil, nil
}
