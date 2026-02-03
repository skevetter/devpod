package dockercredentials

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/stretchr/testify/suite"
)

type HelperTestSuite struct {
	suite.Suite
	helper *Helper
	server *httptest.Server
}

func TestHelperTestSuite(t *testing.T) {
	suite.Run(t, new(HelperTestSuite))
}

func (s *HelperTestSuite) SetupTest() {
	// Create a test HTTP server
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Simulate different scenarios based on server URL
		switch req.ServerURL {
		case "registry.example.com":
			// Return credentials
			creds := Credentials{
				Username: "testuser",
				Secret:   "testpass",
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(creds)
		case "public.registry.com":
			// Return empty credentials (anonymous access)
			creds := Credentials{}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(creds)
		default:
			// Not found
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	// Extract port from test server URL
	serverURL, _ := url.Parse(s.server.URL)
	port, _ := strconv.Atoi(serverURL.Port())
	s.helper = NewHelper(port)
}

func (s *HelperTestSuite) TearDownTest() {
	s.server.Close()
}

func (s *HelperTestSuite) TestGet_WithCredentials() {
	// Test that Get returns credentials from mock server
	username, secret, err := s.helper.Get("registry.example.com")

	// Should not return error
	s.NoError(err)
	// Should return test credentials from mock server
	s.Equal("testuser", username)
	s.Equal("testpass", secret)
}

func (s *HelperTestSuite) TestGet_AnonymousAccess() {
	// Test that empty credentials are returned without error for anonymous access
	username, secret, err := s.helper.Get("public.registry.com")

	// Should not return error - allows anonymous access
	s.NoError(err)
	// Empty credentials returned for anonymous access
	s.Equal("", username)
	s.Equal("", secret)
}

func (s *HelperTestSuite) TestGet_NotFound_ReturnsEmptyCredentials() {
	// When credentials are not found, should return empty credentials (not error)
	// This allows Docker to proceed with anonymous access
	username, secret, err := s.helper.Get("unknown.registry.com")

	// Should not return error
	s.NoError(err)

	// Empty credentials allow anonymous access
	s.Equal("", username)
	s.Equal("", secret)
}

func (s *HelperTestSuite) TestGetFromCredentialsServer_NotFound_ReturnsEmptyCredentials() {
	// Test that getFromCredentialsServer returns empty credentials, not error
	creds, err := s.helper.getFromCredentialsServer("nonexistent.registry.com")

	// Should not return error
	s.NoError(err)

	// Should return empty credentials
	s.NotNil(creds)
	s.Equal("", creds.Username)
	s.Equal("", creds.Secret)
}

func (s *HelperTestSuite) TestList_EmptyList() {
	// List will try to connect to credentials server which isn't running
	// Should return error when server not available
	list, err := s.helper.List()

	// Expect error since credentials server is not available
	s.Error(err)
	// List may be nil or empty when error occurs
	if list != nil {
		s.Empty(list)
	}
}

func (s *HelperTestSuite) TestAdd_NotSupported() {
	// Add should return error as it's not supported
	err := s.helper.Add(nil)

	s.Error(err)
	s.Contains(err.Error(), "not supported")
}

func (s *HelperTestSuite) TestDelete_NotSupported() {
	// Delete should return error as it's not supported
	err := s.helper.Delete("registry.example.com")

	s.Error(err)
	s.Contains(err.Error(), "not supported")
}

func (s *HelperTestSuite) TestGetFromWorkspaceServer_SocketNotExists() {
	// When socket doesn't exist, should return nil (not error)
	creds := s.helper.getFromWorkspaceServer("registry.example.com")

	s.Nil(creds)
}

func (s *HelperTestSuite) TestListFromWorkspaceServer_SocketNotExists() {
	// When socket doesn't exist, should return nil (not error)
	list := s.helper.listFromWorkspaceServer()

	s.Nil(list)
}

func (s *HelperTestSuite) TestGetFromCredentialsServer_MarshalError() {
	// Even with marshal error, should return empty credentials
	creds, err := s.helper.getFromCredentialsServer("")

	s.NoError(err)
	s.NotNil(creds)
	s.Equal("", creds.Username)
	s.Equal("", creds.Secret)
}

func (s *HelperTestSuite) TestGetFromCredentialsServer_ServerError() {
	// When server returns error, should return empty credentials
	creds, err := s.helper.getFromCredentialsServer("error.registry.com")

	s.NoError(err)
	s.NotNil(creds)
	s.Equal("", creds.Username)
	s.Equal("", creds.Secret)
}

func (s *HelperTestSuite) TestGet_EmptyServerURL() {
	// Should handle empty server URL gracefully
	username, secret, err := s.helper.Get("")

	s.NoError(err)
	s.Equal("", username)
	s.Equal("", secret)
}

func (s *HelperTestSuite) TestNewHelper() {
	// Test helper creation
	h := NewHelper(8080)

	s.NotNil(h)
	s.Equal(8080, h.port)
}

func (s *HelperTestSuite) TestRequestWorkspaceList_InvalidResponse() {
	// When workspace server socket doesn't exist, should return error
	listResp, err := s.helper.requestWorkspaceList()

	// Expect error since workspace server socket doesn't exist
	s.Error(err)
	s.Nil(listResp)
}
