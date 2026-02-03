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
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		switch req.ServerURL {
		case "registry.example.com":

			creds := Credentials{
				Username: "testuser",
				Secret:   "testpass",
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(creds)
		case "public.registry.com":

			creds := Credentials{}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(creds)
		case "whitespace.registry.com":

			creds := Credentials{
				Username: "  ",
				Secret:   "  ",
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(creds)
		default:

			w.WriteHeader(http.StatusNotFound)
		}
	}))

	serverURL, _ := url.Parse(s.server.URL)
	port, _ := strconv.Atoi(serverURL.Port())
	s.helper = NewHelper(port)
}

func (s *HelperTestSuite) TearDownTest() {
	s.server.Close()
}

func (s *HelperTestSuite) TestGet_WithCredentials() {
	username, secret, err := s.helper.Get("registry.example.com")

	s.NoError(err)

	s.Equal("testuser", username)
	s.Equal("testpass", secret)
}

func (s *HelperTestSuite) TestGet_AnonymousAccess() {
	username, secret, err := s.helper.Get("public.registry.com")

	s.Error(err)
	s.Contains(err.Error(), "credentials not found")

	s.Equal("", username)
	s.Equal("", secret)
}

func (s *HelperTestSuite) TestGet_NotFound_ReturnsEmptyCredentials() {
	username, secret, err := s.helper.Get("unknown.registry.com")

	s.Error(err)
	s.Contains(err.Error(), "credentials not found")

	s.Equal("", username)
	s.Equal("", secret)
}

func (s *HelperTestSuite) TestGet_WhitespaceCredentials_ReturnsCredentials() {
	username, secret, err := s.helper.Get("whitespace.registry.com")

	s.NoError(err)
	s.Equal("  ", username)
	s.Equal("  ", secret)
}

func (s *HelperTestSuite) TestGetFromCredentialsServer_NotFound_ReturnsEmptyCredentials() {
	creds, err := s.helper.getFromCredentialsServer("nonexistent.registry.com")

	s.Error(err)
	s.Contains(err.Error(), "credentials not found")

	s.Nil(creds)
}

func (s *HelperTestSuite) TestList_EmptyList() {

	list, err := s.helper.List()

	s.Error(err)

	if list != nil {
		s.Empty(list)
	}
}

func (s *HelperTestSuite) TestAdd_NotSupported() {

	err := s.helper.Add(nil)

	s.Error(err)
	s.Contains(err.Error(), "not supported")
}

func (s *HelperTestSuite) TestDelete_NotSupported() {

	err := s.helper.Delete("registry.example.com")

	s.Error(err)
	s.Contains(err.Error(), "not supported")
}

func (s *HelperTestSuite) TestGetFromWorkspaceServer_SocketNotExists() {

	creds := s.helper.getFromWorkspaceServer("registry.example.com")

	s.Nil(creds)
}

func (s *HelperTestSuite) TestListFromWorkspaceServer_SocketNotExists() {

	list := s.helper.listFromWorkspaceServer()

	s.Nil(list)
}

func (s *HelperTestSuite) TestGetFromCredentialsServer_MarshalError() {

	creds, err := s.helper.getFromCredentialsServer("")

	s.Error(err)
	s.Contains(err.Error(), "credentials not found")
	s.Nil(creds)
}

func (s *HelperTestSuite) TestGetFromCredentialsServer_NotFound() {
	creds, err := s.helper.getFromCredentialsServer("error.registry.com")

	s.Error(err)
	s.Contains(err.Error(), "credentials not found")
	s.Nil(creds)
}

func (s *HelperTestSuite) TestGet_EmptyServerURL() {

	username, secret, err := s.helper.Get("")

	s.Error(err)
	s.Contains(err.Error(), "credentials not found")
	s.Equal("", username)
	s.Equal("", secret)
}

func (s *HelperTestSuite) TestNewHelper() {

	h := NewHelper(8080)

	s.NotNil(h)
	s.Equal(8080, h.port)
}

func (s *HelperTestSuite) TestRequestWorkspaceList_InvalidResponse() {

	listResp, err := s.helper.requestWorkspaceList()

	s.Error(err)
	s.Nil(listResp)
}
