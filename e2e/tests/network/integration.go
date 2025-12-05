package network

import (
	"context"
	"time"

	"github.com/loft-sh/log"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/skevetter/devpod/e2e/framework"
	"github.com/skevetter/devpod/pkg/daemon/workspace/network"
)

var _ = DevPodDescribe("network server integration test suite", func() {
	ginkgo.Context("testing network server", ginkgo.Label("network", "server"), func() {
		ginkgo.It("creates and manages network server", func() {
			logger := log.Default.ErrorStreamOnly()
			config := network.ServerConfig{
				Addr:           "localhost:0",
				GRPCTargetAddr: "localhost:50051",
				HTTPTargetAddr: "localhost:8080",
			}
			server := network.NewServer(config, logger)
			gomega.Expect(server).NotTo(gomega.BeNil())

			// Verify components
			tracker := server.Tracker()
			gomega.Expect(tracker).NotTo(gomega.BeNil())
			gomega.Expect(tracker.Count()).To(gomega.Equal(0))

			forwarder := server.Forwarder()
			gomega.Expect(forwarder).NotTo(gomega.BeNil())
			gomega.Expect(len(forwarder.List())).To(gomega.Equal(0))

			netmap := server.NetworkMap()
			gomega.Expect(netmap).NotTo(gomega.BeNil())
			gomega.Expect(netmap.Count()).To(gomega.Equal(0))

			err := server.Stop()
			framework.ExpectNoError(err)
		})

		ginkgo.It("tracks connections through server", func() {
			logger := log.Default.ErrorStreamOnly()
			config := network.ServerConfig{
				Addr: "localhost:0",
			}
			server := network.NewServer(config, logger)
			defer server.Stop()

			tracker := server.Tracker()
			tracker.Add("test-conn", "192.168.1.1:8080")

			gomega.Expect(tracker.Count()).To(gomega.Equal(1))

			conn, exists := tracker.Get("test-conn")
			framework.ExpectEqual(exists, true)
			framework.ExpectEqual(conn.ID, "test-conn")
		})

		ginkgo.It("forwards ports through server", func() {
			logger := log.Default.ErrorStreamOnly()
			config := network.ServerConfig{
				Addr: "localhost:0",
			}
			server := network.NewServer(config, logger)
			defer server.Stop()

			forwarder := server.Forwarder()

			// Get free port
			localPort, err := network.GetFreePort()
			framework.ExpectNoError(err)

			ctx := context.Background()
			err = forwarder.Forward(ctx, string(rune(localPort)), "localhost:8080")
			// Expected to fail if no target server, but should not panic
			if err == nil {
				forwarder.Stop(string(rune(localPort)))
			}
		})

		ginkgo.It("manages network map through server", func() {
			logger := log.Default.ErrorStreamOnly()
			config := network.ServerConfig{
				Addr: "localhost:0",
			}
			server := network.NewServer(config, logger)
			defer server.Stop()

			netmap := server.NetworkMap()
			netmap.AddPeer("peer1", "192.168.1.1:8080")

			gomega.Expect(netmap.Count()).To(gomega.Equal(1))

			peer, exists := netmap.GetPeer("peer1")
			framework.ExpectEqual(exists, true)
			framework.ExpectEqual(peer.ID, "peer1")
		})
	})

	ginkgo.Context("testing end-to-end proxy workflow", ginkgo.Label("e2e"), func() {
		ginkgo.It("completes full proxy workflow", func() {
			logger := log.Default.ErrorStreamOnly()

			// Create server
			config := network.ServerConfig{
				Addr: "localhost:0",
			}
			server := network.NewServer(config, logger)
			defer server.Stop()

			// Add connection
			tracker := server.Tracker()
			tracker.Add("client1", "192.168.1.1:8080")

			// Add peer
			netmap := server.NetworkMap()
			netmap.AddPeer("peer1", "192.168.1.2:8080")

			// Verify state
			gomega.Expect(tracker.Count()).To(gomega.Equal(1))
			gomega.Expect(netmap.Count()).To(gomega.Equal(1))

			// Update connection
			time.Sleep(10 * time.Millisecond)
			tracker.Update("client1")

			conn, exists := tracker.Get("client1")
			framework.ExpectEqual(exists, true)
			gomega.Expect(conn.LastSeen).NotTo(gomega.BeZero())

			// Cleanup
			tracker.Remove("client1")
			netmap.RemovePeer("peer1")

			gomega.Expect(tracker.Count()).To(gomega.Equal(0))
			gomega.Expect(netmap.Count()).To(gomega.Equal(0))
		})
	})
})
