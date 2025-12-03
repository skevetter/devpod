package local

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/loft-sh/log"
	"github.com/skevetter/devpod/pkg/agent/tunnel"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

type mockTunnelClient struct{}

func (m *mockTunnelClient) Ping(ctx context.Context, req *tunnel.Empty, opts ...grpc.CallOption) (*tunnel.Empty, error) {
	return &tunnel.Empty{}, nil
}

func (m *mockTunnelClient) Log(ctx context.Context, msg *tunnel.LogMessage, opts ...grpc.CallOption) (*tunnel.Empty, error) {
	return &tunnel.Empty{}, nil
}

func (m *mockTunnelClient) SendResult(ctx context.Context, msg *tunnel.Message, opts ...grpc.CallOption) (*tunnel.Empty, error) {
	return &tunnel.Empty{}, nil
}

func (m *mockTunnelClient) GitUser(ctx context.Context, req *tunnel.Empty, opts ...grpc.CallOption) (*tunnel.Message, error) {
	return &tunnel.Message{Message: "user@example.com"}, nil
}

func (m *mockTunnelClient) GitCredentials(ctx context.Context, req *tunnel.Message, opts ...grpc.CallOption) (*tunnel.Message, error) {
	return &tunnel.Message{Message: "credentials"}, nil
}

func (m *mockTunnelClient) DockerCredentials(ctx context.Context, req *tunnel.Message, opts ...grpc.CallOption) (*tunnel.Message, error) {
	return &tunnel.Message{Message: "docker-creds"}, nil
}

func (m *mockTunnelClient) GitSSHSignature(ctx context.Context, req *tunnel.Message, opts ...grpc.CallOption) (*tunnel.Message, error) {
	return &tunnel.Message{Message: "signature"}, nil
}

func (m *mockTunnelClient) LoftConfig(ctx context.Context, req *tunnel.Message, opts ...grpc.CallOption) (*tunnel.Message, error) {
	return &tunnel.Message{Message: "config"}, nil
}

func (m *mockTunnelClient) GPGPublicKeys(ctx context.Context, req *tunnel.Message, opts ...grpc.CallOption) (*tunnel.Message, error) {
	return &tunnel.Message{Message: "keys"}, nil
}

func (m *mockTunnelClient) KubeConfig(ctx context.Context, req *tunnel.Message, opts ...grpc.CallOption) (*tunnel.Message, error) {
	return &tunnel.Message{Message: "kubeconfig"}, nil
}

func (m *mockTunnelClient) ForwardPort(ctx context.Context, req *tunnel.ForwardPortRequest, opts ...grpc.CallOption) (*tunnel.ForwardPortResponse, error) {
	return &tunnel.ForwardPortResponse{}, nil
}

func (m *mockTunnelClient) StopForwardPort(ctx context.Context, req *tunnel.StopForwardPortRequest, opts ...grpc.CallOption) (*tunnel.StopForwardPortResponse, error) {
	return &tunnel.StopForwardPortResponse{}, nil
}

func (m *mockTunnelClient) StreamGitClone(ctx context.Context, req *tunnel.Empty, opts ...grpc.CallOption) (grpc.ServerStreamingClient[tunnel.Chunk], error) {
	return nil, nil
}

func (m *mockTunnelClient) StreamWorkspace(ctx context.Context, req *tunnel.Empty, opts ...grpc.CallOption) (grpc.ServerStreamingClient[tunnel.Chunk], error) {
	return nil, nil
}

func (m *mockTunnelClient) StreamMount(ctx context.Context, req *tunnel.StreamMountRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[tunnel.Chunk], error) {
	return nil, nil
}

func TestHTTPTunnelServer(t *testing.T) {
	client := &mockTunnelClient{}
	server := NewHTTPTunnelServer(18080, client, log.Default)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server in background
	go func() { _ = server.Start(ctx) }()
	time.Sleep(100 * time.Millisecond)

	// Test git-user request
	msg := &tunnel.Message{Message: "git-user"}
	body, _ := json.Marshal(msg)
	resp, err := http.Post("http://localhost:18080/", "application/json", bytes.NewReader(body))
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var response tunnel.Message
	_ = json.NewDecoder(resp.Body).Decode(&response)
	assert.Equal(t, "user@example.com", response.Message)
	_ = resp.Body.Close()

	// Stop server
	cancel()
	time.Sleep(100 * time.Millisecond)
}
