package network_test

import (
	"context"
	"testing"

	"github.com/skevetter/devpod/pkg/daemon/workspace/network"
	"github.com/stretchr/testify/suite"
)

type CredentialsProxyTestSuite struct {
	suite.Suite
}

func (s *CredentialsProxyTestSuite) TestProxySendRequest() {
	transport := &network.MockTransport{
		DialFunc: func(ctx context.Context, target string) (network.Conn, error) {
			return &mockConn{}, nil
		},
	}

	proxy := network.NewCredentialsProxy(transport)
	err := proxy.SendRequest(context.Background(), &network.CredentialRequest{
		Service: "git",
	})

	s.NoError(err)
}

func TestCredentialsProxyTestSuite(t *testing.T) {
	suite.Run(t, new(CredentialsProxyTestSuite))
}
