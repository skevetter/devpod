package buildkit

import (
	"context"
	"net"

	"github.com/moby/buildkit/client"
)

type DockerClient interface {
	DialHijack(ctx context.Context, url, proto string, meta map[string][]string) (net.Conn, error)
}

func NewDockerClient(ctx context.Context, dockerClient DockerClient) (*client.Client, error) {
	return client.New(ctx, "", client.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return dockerClient.DialHijack(ctx, "/grpc", "h2c", nil)
	}), client.WithSessionDialer(func(ctx context.Context, proto string, meta map[string][]string) (net.Conn, error) {
		return dockerClient.DialHijack(ctx, "/session", proto, meta)
	}))
}
