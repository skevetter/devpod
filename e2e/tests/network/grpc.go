package network

import (
	"context"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/skevetter/devpod/e2e/framework"
	"github.com/skevetter/devpod/pkg/daemon/workspace/network"
)

var _ = DevPodDescribe("gRPC proxy test suite", func() {
	ginkgo.Context("testing gRPC proxy", ginkgo.Label("network", "grpc"), func() {
		ginkgo.It("creates gRPC proxy", func() {
			config := network.GRPCProxyConfig{
				TargetAddr: "localhost:50051",
			}
			proxy := network.NewGRPCProxy(config)
			gomega.Expect(proxy).NotTo(gomega.BeNil())

			ctx := context.Background()
			err := proxy.Start(ctx)
			framework.ExpectNoError(err)

			server := proxy.Server()
			gomega.Expect(server).NotTo(gomega.BeNil())

			err = proxy.Stop()
			framework.ExpectNoError(err)
		})
	})
})
