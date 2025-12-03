package network

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/suite"
)

type HTTPProxyHandlerTestSuite struct {
	suite.Suite
	handler *HTTPProxyHandler
}

func TestHTTPProxyHandlerTestSuite(t *testing.T) {
	suite.Run(t, new(HTTPProxyHandlerTestSuite))
}

func (s *HTTPProxyHandlerTestSuite) SetupTest() {
	s.handler = NewHTTPProxyHandler("localhost:8080")
}

func (s *HTTPProxyHandlerTestSuite) TestNewHTTPProxyHandler() {
	handler := NewHTTPProxyHandler("localhost:8080")
	s.NotNil(handler)
	s.Equal("localhost:8080", handler.targetAddr)
}

func (s *HTTPProxyHandlerTestSuite) TestServeHTTPWithInvalidTarget() {
	// Create test server
	req := httptest.NewRequest("GET", "http://example.com", nil)
	w := httptest.NewRecorder()

	// Handler should fail gracefully with invalid target
	handler := NewHTTPProxyHandler("invalid:99999")
	handler.ServeHTTP(w, req)

	s.Equal(http.StatusBadGateway, w.Code)
}
