package network

import (
	"io"
	"net"
	"net/http"
	"time"
)

// HTTPProxyHandler handles HTTP proxy requests
type HTTPProxyHandler struct {
	targetAddr string
	timeout    time.Duration
}

// NewHTTPProxyHandler creates a new HTTP proxy handler
func NewHTTPProxyHandler(targetAddr string) *HTTPProxyHandler {
	return &HTTPProxyHandler{
		targetAddr: targetAddr,
		timeout:    30 * time.Second,
	}
}

// ServeHTTP handles HTTP proxy requests
func (h *HTTPProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Dial target
	conn, err := net.DialTimeout("tcp", h.targetAddr, h.timeout)
	if err != nil {
		http.Error(w, "Failed to connect to target", http.StatusBadGateway)
		return
	}
	defer conn.Close()

	// Hijack connection
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, "Failed to hijack connection", http.StatusInternalServerError)
		return
	}
	defer clientConn.Close()

	// Write request to target
	if err := r.Write(conn); err != nil {
		return
	}

	// Bidirectional copy
	done := make(chan error, 2)
	go func() {
		_, err := io.Copy(conn, clientConn)
		done <- err
	}()
	go func() {
		_, err := io.Copy(clientConn, conn)
		done <- err
	}()

	// Wait for completion
	<-done
}
