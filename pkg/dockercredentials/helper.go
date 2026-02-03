package dockercredentials

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/docker/docker-credential-helpers/credentials"
	"github.com/skevetter/devpod/pkg/agent"
	devpodhttp "github.com/skevetter/devpod/pkg/http"
	"github.com/skevetter/devpod/pkg/ts"
)

const (
	workspaceServerURL = "http://runner-proxy/docker-credentials"
	// #nosec G101 -- this is an endpoint path, not a credential
	credentialsEndpoint = "/docker-credentials"
	contentTypeJSON     = "application/json"
	httpClientTimeout   = 15 * time.Second
	// #nosec G101 -- this is a log filename, not a credential
	credentialsErrorLogFile = "docker-credentials-error.log"
	logFilePermissions      = 0644
)

// Helper implements the docker credential helper interface.
type Helper struct {
	port int
}

// NewHelper creates a new credential helper.
func NewHelper(port int) *Helper {
	return &Helper{port: port}
}

// Add stores credentials (not implemented for DevPod).
func (h *Helper) Add(_ *credentials.Credentials) error {
	return fmt.Errorf("storing credentials is not supported")
}

// Delete removes credentials (not implemented for DevPod).
func (h *Helper) Delete(_ string) error {
	return fmt.Errorf("deleting credentials is not supported")
}

// Get retrieves credentials for a server.
func (h *Helper) Get(serverURL string) (string, string, error) {
	// Try workspace server first
	if creds := h.getFromWorkspaceServer(serverURL); creds != nil {
		return creds.Username, creds.Secret, nil
	}

	// Try credentials server
	creds, err := h.getFromCredentialsServer(serverURL)
	if err != nil {
		return "", "", err
	}

	if creds.Username == "" && creds.Secret == "" {
		return "", "", credentials.NewErrCredentialsNotFound()
	}

	return creds.Username, creds.Secret, nil
}

// List returns all stored credentials.
func (h *Helper) List() (map[string]string, error) {
	// Try workspace server first
	if list := h.listFromWorkspaceServer(); list != nil {
		return list, nil
	}

	// Try credentials server
	return h.listFromCredentialsServer()
}

func (h *Helper) getWorkspaceHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", filepath.Join(agent.RootDir, ts.RunnerProxySocket))
			},
		},
		Timeout: httpClientTimeout,
	}
}

func (h *Helper) getFromWorkspaceServer(serverURL string) *Credentials {
	if _, err := os.Stat(filepath.Join(agent.RootDir, ts.RunnerProxySocket)); err != nil {
		return nil
	}

	httpClient := h.getWorkspaceHTTPClient()

	rawJSON, err := json.Marshal(&Request{ServerURL: serverURL})
	if err != nil {
		h.logError("marshal request: %v", err)
		return nil
	}
	response, err := httpClient.Post(
		workspaceServerURL,
		contentTypeJSON,
		bytes.NewReader(rawJSON),
	)
	if err != nil {
		h.logError("get credentials from workspace server: %v", err)
		return nil
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		return nil
	}

	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return nil
	}

	creds := &Credentials{}
	if err := json.Unmarshal(raw, creds); err != nil {
		return nil
	}

	return creds
}

func (h *Helper) listFromWorkspaceServer() map[string]string {
	if _, err := os.Stat(filepath.Join(agent.RootDir, ts.RunnerProxySocket)); err != nil {
		return nil
	}

	httpClient := h.getWorkspaceHTTPClient()

	rawJSON, err := json.Marshal(&Request{})
	if err != nil {
		return nil
	}
	response, err := httpClient.Post(
		workspaceServerURL,
		contentTypeJSON,
		bytes.NewReader(rawJSON),
	)
	if err != nil {
		return nil
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		return nil
	}

	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return nil
	}

	listResp := &ListResponse{}
	if err := json.Unmarshal(raw, listResp); err != nil {
		return nil
	}

	if listResp.Registries == nil {
		return map[string]string{}
	}

	return listResp.Registries
}

func (h *Helper) getFromCredentialsServer(serverURL string) (*Credentials, error) {
	rawJSON, err := json.Marshal(&Request{ServerURL: serverURL})
	if err != nil {
		return nil, credentials.NewErrCredentialsNotFound()
	}

	url := fmt.Sprintf("http://localhost:%d%s", h.port, credentialsEndpoint)
	response, err := devpodhttp.GetHTTPClient().Post(
		url,
		contentTypeJSON,
		bytes.NewReader(rawJSON),
	)
	if err != nil {
		return nil, credentials.NewErrCredentialsNotFound()
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		return nil, credentials.NewErrCredentialsNotFound()
	}

	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, credentials.NewErrCredentialsNotFound()
	}

	creds := &Credentials{}
	if err := json.Unmarshal(raw, creds); err != nil {
		return nil, credentials.NewErrCredentialsNotFound()
	}

	return creds, nil
}

func (h *Helper) listFromCredentialsServer() (map[string]string, error) {
	rawJSON, err := json.Marshal(&Request{})
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("http://localhost:%d%s", h.port, credentialsEndpoint)
	response, err := devpodhttp.GetHTTPClient().Post(
		url,
		contentTypeJSON,
		bytes.NewReader(rawJSON),
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list credentials")
	}

	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	listResp := &ListResponse{}
	if err := json.Unmarshal(raw, listResp); err != nil {
		return nil, err
	}

	if listResp.Registries == nil {
		return map[string]string{}, nil
	}

	return listResp.Registries, nil
}

func (h *Helper) logError(format string, args ...any) {
	logPath := filepath.Join(agent.RootDir, credentialsErrorLogFile)
	// #nosec G302 G304 -- log file needs to be readable for debugging, path is constant
	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, logFilePermissions)
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()

	_, _ = fmt.Fprintf(file, format+"\n", args...)
}
