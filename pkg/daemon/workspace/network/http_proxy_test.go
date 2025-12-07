package network

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/loft-sh/log"
	"github.com/stretchr/testify/suite"
	"tailscale.com/client/tailscale"
	"tailscale.com/tsnet"
)

type HttpProxyHandlerTestSuite struct {
	suite.Suite
	handler *HttpProxyHandler
}

func TestHttpProxyHandlerTestSuite(t *testing.T) {
	suite.Run(t, new(HttpProxyHandlerTestSuite))
}

func (s *HttpProxyHandlerTestSuite) SetupTest() {
	config := &ServerConfig{
		AccessKey: "test-key",
		RootDir:   "/tmp",
	}
	s.handler = NewHttpProxyHandler(&tsnet.Server{}, &tailscale.LocalClient{}, config, "test-project", "test-workspace", log.Default)
}

func (s *HttpProxyHandlerTestSuite) TestNewHttpProxyHandler() {
	config := &ServerConfig{
		AccessKey: "test-key",
		RootDir:   "/tmp",
	}
	handler := NewHttpProxyHandler(&tsnet.Server{}, &tailscale.LocalClient{}, config, "test-project", "test-workspace", log.Default)
	s.NotNil(handler)
	s.Equal("test-project", handler.projectName)
	s.Equal("test-workspace", handler.workspaceName)
}

func (s *HttpProxyHandlerTestSuite) TestServeHTTPWithMissingHeaders() {
	req := httptest.NewRequest("GET", "http://example.com", nil)
	w := httptest.NewRecorder()

	s.handler.ServeHTTP(w, req)

	s.Equal(http.StatusBadRequest, w.Code)
}
