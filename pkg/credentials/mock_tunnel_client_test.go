package credentials

import (
	"context"
	"fmt"

	"github.com/skevetter/devpod/pkg/agent/tunnel"
	"google.golang.org/grpc"
)

type mockTunnelClient struct {
	gitSSHSignatureFunc func(ctx context.Context, msg *tunnel.Message) (*tunnel.Message, error)
}

func (m *mockTunnelClient) Ping(
	ctx context.Context,
	in *tunnel.Empty,
	opts ...grpc.CallOption,
) (*tunnel.Empty, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockTunnelClient) Log(
	ctx context.Context,
	in *tunnel.LogMessage,
	opts ...grpc.CallOption,
) (*tunnel.Empty, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockTunnelClient) SendResult(
	ctx context.Context,
	in *tunnel.Message,
	opts ...grpc.CallOption,
) (*tunnel.Empty, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockTunnelClient) DockerCredentials(
	ctx context.Context,
	in *tunnel.Message,
	opts ...grpc.CallOption,
) (*tunnel.Message, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockTunnelClient) GitCredentials(
	ctx context.Context,
	in *tunnel.Message,
	opts ...grpc.CallOption,
) (*tunnel.Message, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockTunnelClient) GitSSHSignature(
	ctx context.Context,
	in *tunnel.Message,
	opts ...grpc.CallOption,
) (*tunnel.Message, error) {
	return m.gitSSHSignatureFunc(ctx, in)
}

func (m *mockTunnelClient) GitUser(
	ctx context.Context,
	in *tunnel.Empty,
	opts ...grpc.CallOption,
) (*tunnel.Message, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockTunnelClient) LoftConfig(
	ctx context.Context,
	in *tunnel.Message,
	opts ...grpc.CallOption,
) (*tunnel.Message, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockTunnelClient) GPGPublicKeys(
	ctx context.Context,
	in *tunnel.Message,
	opts ...grpc.CallOption,
) (*tunnel.Message, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockTunnelClient) KubeConfig(
	ctx context.Context,
	in *tunnel.Message,
	opts ...grpc.CallOption,
) (*tunnel.Message, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockTunnelClient) ForwardPort(
	ctx context.Context,
	in *tunnel.ForwardPortRequest,
	opts ...grpc.CallOption,
) (*tunnel.ForwardPortResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockTunnelClient) StopForwardPort(
	ctx context.Context,
	in *tunnel.StopForwardPortRequest,
	opts ...grpc.CallOption,
) (*tunnel.StopForwardPortResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockTunnelClient) StreamGitClone(
	ctx context.Context,
	in *tunnel.Empty,
	opts ...grpc.CallOption,
) (grpc.ServerStreamingClient[tunnel.Chunk], error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockTunnelClient) StreamWorkspace(
	ctx context.Context,
	in *tunnel.Empty,
	opts ...grpc.CallOption,
) (grpc.ServerStreamingClient[tunnel.Chunk], error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockTunnelClient) StreamMount(
	ctx context.Context,
	in *tunnel.StreamMountRequest,
	opts ...grpc.CallOption,
) (grpc.ServerStreamingClient[tunnel.Chunk], error) {
	return nil, fmt.Errorf("not implemented")
}
