package local

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/loft-sh/log"
	"github.com/skevetter/devpod/pkg/agent/tunnel"
)

// HTTPTunnelServer forwards HTTP requests to the local tunnel client
type HTTPTunnelServer struct {
	port         int
	tunnelClient tunnel.TunnelClient
	server       *http.Server
	log          log.Logger
}

// NewHTTPTunnelServer creates a new HTTP tunnel server
func NewHTTPTunnelServer(port int, tunnelClient tunnel.TunnelClient, log log.Logger) *HTTPTunnelServer {
	return &HTTPTunnelServer{
		port:         port,
		tunnelClient: tunnelClient,
		log:          log,
	}
}

// Start starts the HTTP tunnel server
func (s *HTTPTunnelServer) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRequest)

	s.server = &http.Server{
		Addr:    ":" + strconv.Itoa(s.port),
		Handler: mux,
	}

	s.log.Infof("Starting HTTP tunnel server on port %d", s.port)

	errChan := make(chan error, 1)
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	select {
	case <-ctx.Done():
		return s.Stop()
	case err := <-errChan:
		return err
	}
}

// Stop stops the HTTP tunnel server
func (s *HTTPTunnelServer) Stop() error {
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.server.Shutdown(ctx)
	}
	return nil
}

// handleRequest forwards requests to the tunnel client
func (s *HTTPTunnelServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.log.Errorf("Failed to read request body: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer func() { _ = r.Body.Close() }()

	// Decode message
	var msg tunnel.Message
	if err := json.Unmarshal(body, &msg); err != nil {
		s.log.Errorf("Failed to decode message: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Route to appropriate handler based on message type
	var response *tunnel.Message
	switch msg.Message {
	case "git-user":
		response, err = s.tunnelClient.GitUser(ctx, &tunnel.Empty{})
	case "git-credentials":
		response, err = s.tunnelClient.GitCredentials(ctx, &msg)
	case "docker-credentials":
		response, err = s.tunnelClient.DockerCredentials(ctx, &msg)
	case "git-ssh-signature":
		response, err = s.tunnelClient.GitSSHSignature(ctx, &msg)
	case "loft-config":
		response, err = s.tunnelClient.LoftConfig(ctx, &msg)
	case "gpg-public-keys":
		response, err = s.tunnelClient.GPGPublicKeys(ctx, &msg)
	case "kube-config":
		response, err = s.tunnelClient.KubeConfig(ctx, &msg)
	default:
		err = fmt.Errorf("unknown message type: %s", msg.Message)
	}

	if err != nil {
		s.log.Errorf("Failed to handle request: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Encode response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.log.Errorf("Failed to encode response: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
