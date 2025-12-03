package network

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/skevetter/devpod/pkg/agent/tunnel"
)

// PlatformCredentialsServer serves platform-specific credentials
type PlatformCredentialsServer struct {
	tunnelClient tunnel.TunnelClient
	mux          *http.ServeMux
}

// NewPlatformCredentialsServer creates a new platform credentials server
func NewPlatformCredentialsServer(tunnelClient tunnel.TunnelClient) *PlatformCredentialsServer {
	s := &PlatformCredentialsServer{
		tunnelClient: tunnelClient,
		mux:          http.NewServeMux(),
	}
	s.registerHandlers()
	return s
}

// ServeHTTP implements http.Handler
func (s *PlatformCredentialsServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// registerHandlers registers HTTP handlers
func (s *PlatformCredentialsServer) registerHandlers() {
	s.mux.HandleFunc("/git-credentials", s.handleGitCredentials)
	s.mux.HandleFunc("/docker-credentials", s.handleDockerCredentials)
	s.mux.HandleFunc("/git-user", s.handleGitUser)
}

// handleGitCredentials handles git credentials requests
func (s *PlatformCredentialsServer) handleGitCredentials(w http.ResponseWriter, r *http.Request) {
	var msg tunnel.Message
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp, err := s.tunnelClient.GitCredentials(r.Context(), &msg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleDockerCredentials handles docker credentials requests
func (s *PlatformCredentialsServer) handleDockerCredentials(w http.ResponseWriter, r *http.Request) {
	var msg tunnel.Message
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp, err := s.tunnelClient.DockerCredentials(r.Context(), &msg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleGitUser handles git user requests
func (s *PlatformCredentialsServer) handleGitUser(w http.ResponseWriter, r *http.Request) {
	resp, err := s.tunnelClient.GitUser(r.Context(), &tunnel.Empty{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// Start starts the credentials server
func (s *PlatformCredentialsServer) Start(ctx context.Context, addr string) error {
	server := &http.Server{
		Addr:    addr,
		Handler: s,
	}

	errChan := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	select {
	case <-ctx.Done():
		return server.Shutdown(context.Background())
	case err := <-errChan:
		return fmt.Errorf("server error: %w", err)
	}
}
