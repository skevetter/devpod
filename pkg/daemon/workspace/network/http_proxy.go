package network

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/loft-sh/log"
	"tailscale.com/client/tailscale"
	"tailscale.com/tsnet"
)

// HttpProxyHandler handles HTTP proxying with Tailscale
type HttpProxyHandler struct {
	tsServer      *tsnet.Server
	lc            *tailscale.LocalClient
	config        *ServerConfig
	projectName   string
	workspaceName string
	log           log.Logger
}

// NewHttpProxyHandler creates HTTP proxy handler
func NewHttpProxyHandler(tsServer *tsnet.Server, lc *tailscale.LocalClient, config *ServerConfig, projectName, workspaceName string, log log.Logger) *HttpProxyHandler {
	return &HttpProxyHandler{
		tsServer:      tsServer,
		lc:            lc,
		config:        config,
		projectName:   projectName,
		workspaceName: workspaceName,
		log:           log,
	}
}

func (h *HttpProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Handle git-credentials endpoint
	if r.URL.Path == "/git-credentials" {
		h.handleGitCredentials(w, r)
		return
	}

	targetHost := r.Header.Get(HeaderTargetHost)
	targetPort := r.Header.Get(HeaderTargetPort)
	if targetHost == "" || targetPort == "" {
		http.Error(w, "missing target headers", http.StatusBadRequest)
		return
	}

	target := net.JoinHostPort(targetHost, targetPort)

	var conn net.Conn
	var err error
	if h.tsServer != nil {
		conn, err = h.tsServer.Dial(r.Context(), "tcp", target)
	} else {
		conn, err = net.Dial("tcp", target)
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("dial: %v", err), http.StatusBadGateway)
		return
	}
	defer func() { _ = conn.Close() }()

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, fmt.Sprintf("hijack: %v", err), http.StatusInternalServerError)
		return
	}
	defer func() { _ = clientConn.Close() }()

	if err := r.Write(conn); err != nil {
		return
	}

	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(conn, clientConn)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(clientConn, conn)
		done <- struct{}{}
	}()

	<-done
}

func (h *HttpProxyHandler) handleGitCredentials(w http.ResponseWriter, r *http.Request) {
	h.log.Infof("HttpProxyHandler: received git credentials request from %s", r.RemoteAddr)

	discoveredRunner, err := discoverRunner(r.Context(), h.lc, h.log)
	if err != nil {
		http.Error(w, "failed to discover runner", http.StatusInternalServerError)
		return
	}

	runnerURL := fmt.Sprintf("http://%s.ts.loft/devpod/%s/%s/workspace-git-credentials", discoveredRunner, h.projectName, h.workspaceName)
	parsedURL, err := http.NewRequest(r.Method, runnerURL, r.Body)
	if err != nil {
		http.Error(w, "failed to create request", http.StatusInternalServerError)
		return
	}

	parsedURL.Header = r.Header.Clone()
	parsedURL.Header.Set("Authorization", "Bearer "+h.config.AccessKey)

	client := &http.Client{
		Transport: &http.Transport{DialContext: h.tsServer.Dial},
		Timeout:   30 * time.Second,
	}

	resp, err := client.Do(parsedURL)
	if err != nil {
		http.Error(w, fmt.Sprintf("request failed: %v", err), http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
