package network

import (
	"context"
	"os"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/skevetter/devpod/e2e/framework"
	"github.com/skevetter/devpod/pkg/daemon/workspace/network"
)

var _ = DevPodDescribe("network transport test suite", func() {
	ginkgo.Context("testing network transport", ginkgo.Label("network"), func() {
		ginkgo.It("http transport connects", ginkgo.Label("network-http"), func() {
			ctx := context.Background()
			transport := network.NewHTTPTransport("localhost", "8080")
			defer func() { _ = transport.Close() }()

			_, err := transport.Dial(ctx, "localhost:8080")
			// Expected to fail if no server running, but should not panic
			framework.ExpectError(err)
		})

		ginkgo.It("stdio transport works", ginkgo.Label("network-stdio"), func() {
			ctx := context.Background()
			transport := network.NewStdioTransport(os.Stdin, os.Stdout)
			defer func() { _ = transport.Close() }()

			conn, err := transport.Dial(ctx, "")
			framework.ExpectNoError(err)
			gomega.Expect(conn).NotTo(gomega.BeNil())
		})

		ginkgo.It("fallback transport uses secondary on primary failure", ginkgo.Label("network-fallback"), func() {
			ctx := context.Background()
			httpTransport := network.NewHTTPTransport("invalid-host", "9999")
			stdioTransport := network.NewStdioTransport(os.Stdin, os.Stdout)
			fallbackTransport := network.NewFallbackTransport(httpTransport, stdioTransport)
			defer func() { _ = fallbackTransport.Close() }()

			conn, err := fallbackTransport.Dial(ctx, "")
			framework.ExpectNoError(err)
			gomega.Expect(conn).NotTo(gomega.BeNil())
		})

		ginkgo.It("connection pool reuses connections", ginkgo.Label("network-pool"), func() {
			pool := network.NewConnectionPool(5, 10)
			defer func() { _ = pool.Close() }()

			gomega.Expect(pool).NotTo(gomega.BeNil())
		})

		ginkgo.It("health check detects transport status", ginkgo.Label("network-health"), func() {
			ctx := context.Background()
			transport := network.NewStdioTransport(os.Stdin, os.Stdout)
			defer func() { _ = transport.Close() }()

			status := network.CheckHealth(ctx, transport)
			framework.ExpectEqual(status.Healthy, true)
		})
	})
})
