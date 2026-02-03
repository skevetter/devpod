package dockercredentials

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/docker/docker-credential-helpers/credentials"
	"github.com/stretchr/testify/suite"
)

type HelperTestSuite struct {
	suite.Suite
}

func TestHelperSuite(t *testing.T) {
	suite.Run(t, new(HelperTestSuite))
}

func (s *HelperTestSuite) TestGet_Success() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/credentials" {
			json.NewEncoder(w).Encode(map[string]string{
				"username": "testuser",
				"secret":   "testpass",
			})
		}
	}))
	defer server.Close()

	helper := NewHelper(parsePort(server.URL))
	username, secret, err := helper.Get("docker.io")

	s.NoError(err)
	s.Equal("testuser", username)
	s.Equal("testpass", secret)
}

func (s *HelperTestSuite) TestGet_EmptyCredentials() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	helper := NewHelper(parsePort(server.URL))
	username, secret, err := helper.Get("docker.io")

	s.NoError(err)
	s.Empty(username)
	s.Empty(secret)
}

func (s *HelperTestSuite) TestGet_ServerError() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	helper := NewHelper(parsePort(server.URL))
	username, secret, err := helper.Get("docker.io")

	s.NoError(err)
	s.Empty(username)
	s.Empty(secret)
}

func (s *HelperTestSuite) TestGet_WorkspaceServerFallback() {
	primaryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer primaryServer.Close()

	workspaceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{
			"username": "workspaceuser",
			"secret":   "workspacepass",
		})
	}))
	defer workspaceServer.Close()

	os.Setenv("DEVPOD_WORKSPACE_CREDENTIALS_PORT", parsePortString(workspaceServer.URL))
	defer os.Unsetenv("DEVPOD_WORKSPACE_CREDENTIALS_PORT")

	helper := NewHelper(parsePort(primaryServer.URL))
	username, secret, err := helper.Get("docker.io")

	s.NoError(err)
	s.Equal("workspaceuser", username)
	s.Equal("workspacepass", secret)
}

func (s *HelperTestSuite) TestList_Success() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/list" {
			json.NewEncoder(w).Encode(map[string]string{
				"docker.io": "testuser",
				"gcr.io":    "gcpuser",
			})
		}
	}))
	defer server.Close()

	helper := NewHelper(parsePort(server.URL))
	list, err := helper.List()

	s.NoError(err)
	s.Len(list, 2)
	s.Equal("testuser", list["docker.io"])
}

func (s *HelperTestSuite) TestList_Empty() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	helper := NewHelper(parsePort(server.URL))
	list, err := helper.List()

	s.NoError(err)
	s.Empty(list)
}

func (s *HelperTestSuite) TestAdd_NotSupported() {
	helper := NewHelper(0)
	err := helper.Add(&credentials.Credentials{})
	s.Error(err)
}

func (s *HelperTestSuite) TestDelete_NotSupported() {
	helper := NewHelper(0)
	err := helper.Delete("docker.io")
	s.Error(err)
}

func (s *HelperTestSuite) TestSanitizeServerURL() {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://docker.io", "docker.io"},
		{"http://docker.io", "docker.io"},
		{"docker.io/", "docker.io"},
		{"https://docker.io/", "docker.io"},
		{"docker.io", "docker.io"},
	}

	for _, tt := range tests {
		result := sanitizeServerURL(tt.input)
		s.Equal(tt.expected, result)
	}
}

func parsePort(url string) int {
	var port int
	fmt.Sscanf(url, "http://127.0.0.1:%d", &port)
	return port
}

func parsePortString(url string) string {
	return fmt.Sprintf("%d", parsePort(url))
}
