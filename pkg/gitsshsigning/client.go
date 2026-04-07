package gitsshsigning

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/skevetter/devpod/pkg/config"
	devpodhttp "github.com/skevetter/devpod/pkg/http"
	"github.com/skevetter/log"
)

const defaultCredentialsServerPort = "12049"

func getCredentialsPort() (int, error) {
	strPort := os.Getenv(config.EnvCredentialsServerPort)
	if strPort == "" {
		strPort = defaultCredentialsServerPort
	}
	port, err := strconv.Atoi(strPort)
	if err != nil {
		return 0, fmt.Errorf("convert port %s: %w", strPort, err)
	}
	return port, nil
}

// HandleGitSSHProgramCall implements logic handling call from git when signing a commit.
func HandleGitSSHProgramCall(certPath, namespace, bufferFile string, log log.Logger) error {
	content, err := extractContentFromGitBuffer(bufferFile)
	if err != nil {
		return err
	}

	signature, err := requestContentSignature(content, certPath, log)
	if err != nil {
		return err
	}

	if err := writeSignatureToFile(signature, bufferFile, log); err != nil {
		return err
	}

	return nil
}

// extractContentFromGitBuffer reads the content from the buffer file created by git.
func extractContentFromGitBuffer(bufferFile string) ([]byte, error) {
	return os.ReadFile(bufferFile)
}

// requestContentSignature sends an HTTP request to the credentials server to sign the content.
func requestContentSignature(content []byte, certPath string, log log.Logger) ([]byte, error) {
	requestBody, err := createSignatureRequestBody(content, certPath)
	if err != nil {
		return nil, err
	}

	responseBody, err := sendSignatureRequest(requestBody, log)
	if err != nil {
		return nil, err
	}

	return parseSignatureResponse(responseBody, log)
}

// writeSignatureToFile writes the signed content to a .sig file.
func writeSignatureToFile(signature []byte, bufferFile string, log log.Logger) error {
	sigFile := bufferFile + ".sig"
	// #nosec G306 -- TODO Consider using a more secure permission setting and ownership if needed.
	if err := os.WriteFile(sigFile, signature, 0o644); err != nil {
		log.Errorf("Failed to write signature to file: %v", err)
		return err
	}
	return nil
}

func createSignatureRequestBody(content []byte, certPath string) ([]byte, error) {
	request := &GitSSHSignatureRequest{
		Content: string(content),
		KeyPath: certPath,
	}
	return json.Marshal(request)
}

// signatureServerURL overrides the server URL for testing. Empty means use credentials.GetPort().
var signatureServerURL string

// SetSignatureServerURL sets the server URL override for testing.
func SetSignatureServerURL(url string) {
	signatureServerURL = url
}

func getSignatureURL() (string, error) {
	if signatureServerURL != "" {
		return signatureServerURL, nil
	}
	port, err := getCredentialsPort()
	if err != nil {
		return "", err
	}
	return "http://localhost:" + strconv.Itoa(port) + "/git-ssh-signature", nil
}

func sendSignatureRequest(requestBody []byte, log log.Logger) ([]byte, error) {
	url, err := getSignatureURL()
	if err != nil {
		return nil, err
	}

	response, err := devpodhttp.GetHTTPClient().Post(
		url,
		"application/json",
		bytes.NewReader(requestBody),
	)
	if err != nil {
		log.Errorf("Error retrieving git ssh signature: %v", err)
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("reading signature response: %w", err)
	}

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"signature server returned %d: %s",
			response.StatusCode,
			strings.TrimSpace(string(body)),
		)
	}

	return body, nil
}

func parseSignatureResponse(responseBody []byte, log log.Logger) ([]byte, error) {
	signatureResponse := &GitSSHSignatureResponse{}
	if err := json.Unmarshal(responseBody, signatureResponse); err != nil {
		log.Errorf("Error decoding git ssh signature: %v", err)
		return nil, fmt.Errorf(
			"error decoding signature response (body: %s): %w",
			string(responseBody),
			err,
		)
	}

	return signatureResponse.Signature, nil
}
