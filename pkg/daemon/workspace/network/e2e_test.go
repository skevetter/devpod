package network_test

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/skevetter/devpod/pkg/daemon/workspace/network"
	"github.com/stretchr/testify/suite"
)

type E2ETestSuite struct {
	suite.Suite
}

func (s *E2ETestSuite) TestHTTPTunnelCredentials() {
	// Start test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("credentials"))
	}))
	defer server.Close()

	// Parse server URL
	u, _ := url.Parse(server.URL)
	host, port, _ := net.SplitHostPort(u.Host)

	// Create HTTP transport
	transport := network.NewHTTPTransport(host, port)

	// Create credentials proxy
	proxy := network.NewCredentialsProxy(transport)

	// Send request
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := proxy.SendRequest(ctx, &network.CredentialRequest{
		Service: "git",
	})

	s.NoError(err)
}

func (s *E2ETestSuite) TestFallbackToStdio() {
	// Create HTTP transport that will fail
	httpTransport := network.NewHTTPTransport("invalid-host", "99999")

	// Create stdio transport as fallback
	stdin := bytes.NewReader([]byte("test"))
	stdout := &bytes.Buffer{}
	stdioTransport := network.NewStdioTransport(stdin, stdout)

	// Create fallback transport
	transport := network.NewFallbackTransport(httpTransport, stdioTransport)

	// Dial should fallback to stdio
	ctx := context.Background()
	conn, err := transport.Dial(ctx, "")

	s.NoError(err)
	s.NotNil(conn)
}

func (s *E2ETestSuite) TestConnectionPooling() {
	// Start test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Parse server URL
	u, _ := url.Parse(server.URL)
	host, port, _ := net.SplitHostPort(u.Host)

	// Create pool
	pool := network.NewConnectionPool(5, 10)
	defer pool.Close()

	// Create transport
	transport := network.NewHTTPTransport(host, port)

	// Get connection
	conn1, err := transport.Dial(context.Background(), "")
	s.NoError(err)
	s.NotNil(conn1)

	// Put back in pool
	err = pool.Put(conn1)
	s.NoError(err)

	// Get again - should reuse
	conn2, err := pool.Get(context.Background())
	s.NoError(err)
	s.Equal(conn1, conn2)
}

func TestE2ETestSuite(t *testing.T) {
	suite.Run(t, new(E2ETestSuite))
}
