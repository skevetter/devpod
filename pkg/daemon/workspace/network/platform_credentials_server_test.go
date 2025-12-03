package network

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/skevetter/devpod/pkg/agent/tunnel"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
)

type mockTunnelClient struct{}

func (m *mockTunnelClient) GitCredentials(ctx context.Context, msg *tunnel.Message, opts ...grpc.CallOption) (*tunnel.Message, error) {
	return &tunnel.Message{Message: "credentials"}, nil
}

func (m *mockTunnelClient) DockerCredentials(ctx context.Context, msg *tunnel.Message, opts ...grpc.CallOption) (*tunnel.Message, error) {
	return &tunnel.Message{Message: "docker-creds"}, nil
}

func (m *mockTunnelClient) GitUser(ctx context.Context, empty *tunnel.Empty, opts ...grpc.CallOption) (*tunnel.Message, error) {
	return &tunnel.Message{Message: "user"}, nil
}

func (m *mockTunnelClient) Ping(ctx context.Context, empty *tunnel.Empty, opts ...grpc.CallOption) (*tunnel.Empty, error) {
	return &tunnel.Empty{}, nil
}

func (m *mockTunnelClient) Log(ctx context.Context, msg *tunnel.LogMessage, opts ...grpc.CallOption) (*tunnel.Empty, error) {
	return &tunnel.Empty{}, nil
}

func (m *mockTunnelClient) SendResult(ctx context.Context, msg *tunnel.Message, opts ...grpc.CallOption) (*tunnel.Empty, error) {
	return &tunnel.Empty{}, nil
}

func (m *mockTunnelClient) GitSSHSignature(ctx context.Context, msg *tunnel.Message, opts ...grpc.CallOption) (*tunnel.Message, error) {
	return &tunnel.Message{}, nil
}

func (m *mockTunnelClient) ForwardPort(ctx context.Context, req *tunnel.ForwardPortRequest, opts ...grpc.CallOption) (*tunnel.ForwardPortResponse, error) {
	return &tunnel.ForwardPortResponse{}, nil
}

func (m *mockTunnelClient) StopForwardPort(ctx context.Context, req *tunnel.StopForwardPortRequest, opts ...grpc.CallOption) (*tunnel.StopForwardPortResponse, error) {
	return &tunnel.StopForwardPortResponse{}, nil
}

func (m *mockTunnelClient) LoftConfig(ctx context.Context, msg *tunnel.Message, opts ...grpc.CallOption) (*tunnel.Message, error) {
	return &tunnel.Message{}, nil
}

func (m *mockTunnelClient) GPGPublicKeys(ctx context.Context, msg *tunnel.Message, opts ...grpc.CallOption) (*tunnel.Message, error) {
	return &tunnel.Message{}, nil
}

func (m *mockTunnelClient) KubeConfig(ctx context.Context, msg *tunnel.Message, opts ...grpc.CallOption) (*tunnel.Message, error) {
	return &tunnel.Message{}, nil
}

func (m *mockTunnelClient) StreamGitClone(ctx context.Context, empty *tunnel.Empty, opts ...grpc.CallOption) (grpc.ServerStreamingClient[tunnel.Chunk], error) {
	return nil, nil
}

func (m *mockTunnelClient) StreamWorkspace(ctx context.Context, empty *tunnel.Empty, opts ...grpc.CallOption) (grpc.ServerStreamingClient[tunnel.Chunk], error) {
	return nil, nil
}

func (m *mockTunnelClient) StreamMount(ctx context.Context, req *tunnel.StreamMountRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[tunnel.Chunk], error) {
	return nil, nil
}

type PlatformCredentialsServerTestSuite struct {
	suite.Suite
	server *PlatformCredentialsServer
}

func TestPlatformCredentialsServerTestSuite(t *testing.T) {
	suite.Run(t, new(PlatformCredentialsServerTestSuite))
}

func (s *PlatformCredentialsServerTestSuite) SetupTest() {
	s.server = NewPlatformCredentialsServer(&mockTunnelClient{})
}

func (s *PlatformCredentialsServerTestSuite) TestNewPlatformCredentialsServer() {
	server := NewPlatformCredentialsServer(&mockTunnelClient{})
	s.NotNil(server)
}

func (s *PlatformCredentialsServerTestSuite) TestHandleGitCredentials() {
	msg := &tunnel.Message{Message: "test"}
	body, _ := json.Marshal(msg)
	req := httptest.NewRequest("POST", "/git-credentials", bytes.NewReader(body))
	w := httptest.NewRecorder()

	s.server.handleGitCredentials(w, req)

	s.Equal(http.StatusOK, w.Code)
	var resp tunnel.Message
	json.NewDecoder(w.Body).Decode(&resp)
	s.Equal("credentials", resp.Message)
}

func (s *PlatformCredentialsServerTestSuite) TestHandleDockerCredentials() {
	msg := &tunnel.Message{Message: "test"}
	body, _ := json.Marshal(msg)
	req := httptest.NewRequest("POST", "/docker-credentials", bytes.NewReader(body))
	w := httptest.NewRecorder()

	s.server.handleDockerCredentials(w, req)

	s.Equal(http.StatusOK, w.Code)
	var resp tunnel.Message
	json.NewDecoder(w.Body).Decode(&resp)
	s.Equal("docker-creds", resp.Message)
}

func (s *PlatformCredentialsServerTestSuite) TestHandleGitUser() {
	req := httptest.NewRequest("GET", "/git-user", nil)
	w := httptest.NewRecorder()

	s.server.handleGitUser(w, req)

	s.Equal(http.StatusOK, w.Code)
	var resp tunnel.Message
	json.NewDecoder(w.Body).Decode(&resp)
	s.Equal("user", resp.Message)
}
