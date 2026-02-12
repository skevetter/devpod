package dockercredentials

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker-credential-helpers/credentials"
)

const (
	credentialsTimeout = 5 * time.Second
	logFileName        = "credential-helper.log"
)

// Helper implements the Docker credential helper interface.
type Helper struct {
	port int
}

// NewHelper creates a new credential helper.
func NewHelper(port int) *Helper {
	return &Helper{port: port}
}

// Add is not supported (read-only helper).
func (h *Helper) Add(*credentials.Credentials) error {
	return credentials.NewErrCredentialsNotFound()
}

// Delete is not supported (read-only helper).
func (h *Helper) Delete(string) error {
	return credentials.NewErrCredentialsNotFound()
}

// Get retrieves credentials for the given server URL.
func (h *Helper) Get(serverURL string) (string, string, error) {
	serverURL = sanitizeServerURL(serverURL)

	// Try primary credential server
	username, secret, err := h.getFromCredentialsServer(serverURL)
	if err == nil && username != "" {
		return username, secret, nil
	}

	// Try workspace server fallback (for Tailscale environments)
	username, secret, err = h.getFromWorkspaceServer(serverURL)
	if err == nil && username != "" {
		return username, secret, nil
	}

	// Return empty credentials for anonymous access
	return "", "", nil
}

// List returns all configured registries.
func (h *Helper) List() (map[string]string, error) {
	payload, err := json.Marshal(&Request{})
	if err != nil {
		h.logError("marshal list request", err)
		return map[string]string{}, nil
	}

	client := &http.Client{Timeout: credentialsTimeout}
	resp, err := client.Post(fmt.Sprintf("http://localhost:%d/docker-credentials", h.port), "application/json", bytes.NewReader(payload))
	if err != nil {
		h.logError("list registries", err)
		return map[string]string{}, nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return map[string]string{}, nil
	}

	var response ListResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		h.logError("decode list response", err)
		return map[string]string{}, nil
	}

	if response.Registries == nil {
		return map[string]string{}, nil
	}

	return response.Registries, nil
}

func (h *Helper) getFromCredentialsServer(serverURL string) (string, string, error) {
	requestBody, err := json.Marshal(&Request{ServerURL: serverURL})
	if err != nil {
		return "", "", err
	}

	client := &http.Client{Timeout: credentialsTimeout}
	resp, err := client.Post(fmt.Sprintf("http://localhost:%d/docker-credentials", h.port), "application/json", bytes.NewReader(requestBody))
	if err != nil {
		return "", "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("status %d", resp.StatusCode)
	}

	var creds Credentials
	if err := json.NewDecoder(resp.Body).Decode(&creds); err != nil {
		return "", "", err
	}

	return creds.Username, creds.Secret, nil
}

func (h *Helper) getFromWorkspaceServer(serverURL string) (string, string, error) {
	workspacePort := os.Getenv("DEVPOD_WORKSPACE_CREDENTIALS_PORT")
	if workspacePort == "" {
		return "", "", fmt.Errorf("no workspace port")
	}

	requestBody, err := json.Marshal(&Request{ServerURL: serverURL})
	if err != nil {
		return "", "", err
	}

	client := &http.Client{Timeout: credentialsTimeout}
	resp, err := client.Post(fmt.Sprintf("http://localhost:%s/docker-credentials", workspacePort), "application/json", bytes.NewReader(requestBody))
	if err != nil {
		return "", "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("status %d", resp.StatusCode)
	}

	var creds Credentials
	if err := json.NewDecoder(resp.Body).Decode(&creds); err != nil {
		return "", "", err
	}

	return creds.Username, creds.Secret, nil
}

func sanitizeServerURL(serverURL string) string {
	serverURL = strings.TrimPrefix(serverURL, "https://")
	serverURL = strings.TrimPrefix(serverURL, "http://")
	serverURL = strings.TrimSuffix(serverURL, "/")
	return serverURL
}

func (h *Helper) logError(operation string, err error) {
	logPath := filepath.Join(os.TempDir(), logFileName)
	// #nosec G304 -- log file path is controlled and in temp directory
	f, ferr := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if ferr != nil {
		return
	}
	defer func() { _ = f.Close() }()

	timestamp := time.Now().Format(time.RFC3339)
	_, _ = fmt.Fprintf(f, "[%s] %s: %v\n", timestamp, operation, err)
}
