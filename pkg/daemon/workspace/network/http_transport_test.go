package network_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/skevetter/devpod/pkg/daemon/workspace/network"
	"github.com/stretchr/testify/suite"
)

type HTTPTransportTestSuite struct {
	suite.Suite
}

func (s *HTTPTransportTestSuite) TestNewHTTPTransport() {
	transport := network.NewHTTPTransport("localhost", "8080")
	s.NotNil(transport)
}

func (s *HTTPTransportTestSuite) TestHTTPTransportDial() {
	// Start test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Parse server URL
	u, _ := url.Parse(server.URL)
	host, port, _ := net.SplitHostPort(u.Host)

	transport := network.NewHTTPTransport(host, port)
	conn, err := transport.Dial(context.Background(), "")

	s.NoError(err)
	s.NotNil(conn)
	_ = conn.Close()
}

func TestHTTPTransportTestSuite(t *testing.T) {
	suite.Run(t, new(HTTPTransportTestSuite))
}
