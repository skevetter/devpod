package network

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"

	"github.com/loft-sh/log"
	"tailscale.com/client/tailscale"
)

// ParseHostPort parses a host:port string
func ParseHostPort(addr string) (host string, port int, err error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return "", 0, err
	}
	port, err = strconv.Atoi(portStr)
	if err != nil {
		return "", 0, fmt.Errorf("invalid port: %w", err)
	}
	return host, port, nil
}

// FormatHostPort formats a host and port into a string
func FormatHostPort(host string, port int) string {
	return net.JoinHostPort(host, strconv.Itoa(port))
}

// IsLocalhost checks if an address is localhost
func IsLocalhost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// GetFreePort finds an available port
func GetFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}
	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// newReverseProxy creates a reverse proxy to the target and applies header modifications.
func newReverseProxy(target *url.URL, modifyHeaders func(http.Header)) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Director = func(req *http.Request) {
		dest := *target
		req.URL = &dest
		req.Host = dest.Host
		modifyHeaders(req.Header)
	}
	return proxy
}

// discoverRunner finds a peer whose hostname ends with "runner".
func discoverRunner(ctx context.Context, lc *tailscale.LocalClient, log log.Logger) (string, error) {
	status, err := lc.Status(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get status: %w", err)
	}
	var runner string
	for _, peer := range status.Peer {
		if peer == nil || peer.HostName == "" {
			continue
		}
		if strings.HasSuffix(peer.HostName, "runner") {
			runner = peer.HostName
			break
		}
	}
	if runner == "" {
		return "", fmt.Errorf("no active runner found")
	}
	log.Infof("discoverRunner: selected runner = %s", runner)
	return runner, nil
}
