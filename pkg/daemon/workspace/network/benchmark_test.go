package network_test

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/skevetter/devpod/pkg/daemon/workspace/network"
)

func BenchmarkHTTPTransportDial(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	u, _ := url.Parse(server.URL)
	host, port, _ := net.SplitHostPort(u.Host)
	transport := network.NewHTTPTransport(host, port)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn, err := transport.Dial(context.Background(), "")
		if err != nil {
			b.Fatal(err)
		}
		conn.Close()
	}
}

func BenchmarkStdioTransportDial(b *testing.B) {
	stdin := bytes.NewReader(make([]byte, 1024))
	stdout := &bytes.Buffer{}
	transport := network.NewStdioTransport(stdin, stdout)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn, err := transport.Dial(context.Background(), "")
		if err != nil {
			b.Fatal(err)
		}
		conn.Close()
	}
}

func BenchmarkConnectionPoolGetPut(b *testing.B) {
	pool := network.NewConnectionPool(10, 50)
	defer pool.Close()

	// Pre-populate pool
	for i := 0; i < 10; i++ {
		pool.Put(&testConn{id: string(rune(i))})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn, err := pool.Get(context.Background())
		if err != nil {
			b.Fatal(err)
		}
		pool.Put(conn)
	}
}

func BenchmarkCredentialsProxySendRequest(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	u, _ := url.Parse(server.URL)
	host, port, _ := net.SplitHostPort(u.Host)
	transport := network.NewHTTPTransport(host, port)
	proxy := network.NewCredentialsProxy(transport)

	req := &network.CredentialRequest{Service: "git"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := proxy.SendRequest(context.Background(), req)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFallbackTransportHTTPSuccess(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	u, _ := url.Parse(server.URL)
	host, port, _ := net.SplitHostPort(u.Host)
	httpTransport := network.NewHTTPTransport(host, port)

	stdin := bytes.NewReader(make([]byte, 1024))
	stdout := &bytes.Buffer{}
	stdioTransport := network.NewStdioTransport(stdin, stdout)

	transport := network.NewFallbackTransport(httpTransport, stdioTransport)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn, err := transport.Dial(context.Background(), "")
		if err != nil {
			b.Fatal(err)
		}
		conn.Close()
	}
}

func BenchmarkFallbackTransportHTTPFail(b *testing.B) {
	// HTTP will fail, should fallback to stdio
	httpTransport := network.NewHTTPTransport("invalid-host", "99999")

	stdin := bytes.NewReader(make([]byte, 1024))
	stdout := &bytes.Buffer{}
	stdioTransport := network.NewStdioTransport(stdin, stdout)

	transport := network.NewFallbackTransport(httpTransport, stdioTransport)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn, err := transport.Dial(context.Background(), "")
		if err != nil {
			b.Fatal(err)
		}
		conn.Close()
	}
}
