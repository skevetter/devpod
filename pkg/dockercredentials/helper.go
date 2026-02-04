package dockercredentials

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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

// Delete is not supported (read-only helper)
func (h *Helper) Delete(string) error {
	return credentials.NewErrCredentialsNotFound()
}

// Get retrieves credentials for the given server URL
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

// List returns all configured registries
func (h *Helper) List() (map[string]string, error) {
	client := &http.Client{Timeout: credentialsTimeout}
	resp, err := client.Get(fmt.Sprintf("http://localhost:%d/list", h.port))
	if err != nil {
		h.logError("list registries", err)
		return map[string]string{}, nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return map[string]string{}, nil
	}

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		h.logError("decode list response", err)
		return map[string]string{}, nil
	}

	return result, nil
}

func (h *Helper) getFromCredentialsServer(serverURL string) (string, string, error) {
	client := &http.Client{Timeout: credentialsTimeout}
	u := url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("localhost:%d", h.port),
		Path:   "/credentials",
		RawQuery: url.Values{
			"registry": []string{serverURL},
		}.Encode(),
	}
	reqURL := u.String()
	resp, err := client.Get(reqURL)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("status %d", resp.StatusCode)
	}

	var creds struct {
		Username string `json:"username"`
		Secret   string `json:"secret"`
	}
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

	client := &http.Client{Timeout: credentialsTimeout}
	u := url.URL{
		Scheme:   "http",
		Host:     fmt.Sprintf("localhost:%s", workspacePort),
		Path:     "/credentials",
		RawQuery: url.Values{"registry": []string{serverURL}}.Encode(),
	}
	resp, err := client.Get(u.String())
	if err != nil {
		return "", "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("status %d", resp.StatusCode)
	}

	var creds struct {
		Username string `json:"username"`
		Secret   string `json:"secret"`
	}
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
