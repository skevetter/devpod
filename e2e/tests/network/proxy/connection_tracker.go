package proxy

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/loft-sh/log"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/skevetter/devpod/e2e/framework"
	"github.com/skevetter/devpod/pkg/daemon/workspace/network"
)

var _ = DevPodDescribe("network proxy test suite", func() {
	ginkgo.Context("testing connection tracker", ginkgo.Label("proxy"), func() {
		ginkgo.It("tracks connections", func() {
			tracker := network.NewConnectionTracker()

			tracker.Add("conn1", "192.168.1.1:8080")
			tracker.Add("conn2", "192.168.1.2:8080")

			gomega.Expect(tracker.Count()).To(gomega.Equal(2))

			conn, exists := tracker.Get("conn1")
			framework.ExpectEqual(exists, true)
			framework.ExpectEqual(conn.ID, "conn1")

			tracker.Remove("conn1")
			gomega.Expect(tracker.Count()).To(gomega.Equal(1))
		})
	})

	ginkgo.Context("testing heartbeat system", ginkgo.Label("proxy"), func() {
		ginkgo.It("removes stale connections", func() {
			tracker := network.NewConnectionTracker()
			config := network.HeartbeatConfig{
				Interval: 50 * time.Millisecond,
				Timeout:  100 * time.Millisecond,
			}
			hb := network.NewHeartbeat(config, tracker)

			tracker.Add("conn1", "192.168.1.1:8080")
			gomega.Expect(tracker.Count()).To(gomega.Equal(1))

			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()

			go hb.Start(ctx)
			time.Sleep(150 * time.Millisecond)

			gomega.Expect(tracker.Count()).To(gomega.Equal(0))
			hb.Stop()
		})
	})

	ginkgo.Context("testing port forwarding", ginkgo.Label("proxy"), func() {
		ginkgo.It("forwards ports", func() {
			logger := log.Default.ErrorStreamOnly()
			forwarder := network.NewPortForwarder(logger)

			// Create test server
			listener, err := net.Listen("tcp", "localhost:0")
			framework.ExpectNoError(err)
			defer listener.Close()

			serverAddr := listener.Addr().String()
			go func() {
				conn, _ := listener.Accept()
				if conn != nil {
					conn.Write([]byte("hello"))
					conn.Close()
				}
			}()

			// Get free port for forwarding
			localPort, err := network.GetFreePort()
			framework.ExpectNoError(err)

			ctx := context.Background()
			err = forwarder.Forward(ctx, fmt.Sprintf("%d", localPort), serverAddr)
			framework.ExpectNoError(err)
			defer forwarder.Stop(fmt.Sprintf("%d", localPort))

			time.Sleep(50 * time.Millisecond)

			// Connect to forwarded port
			conn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", localPort))
			if err == nil {
				defer conn.Close()
				buf := make([]byte, 5)
				conn.Read(buf)
				framework.ExpectEqual(string(buf), "hello")
			}
		})
	})

	ginkgo.Context("testing SSH tunnel", ginkgo.Label("proxy"), func() {
		ginkgo.It("creates tunnel", func() {
			logger := log.Default.ErrorStreamOnly()

			// Create test server
			listener, err := net.Listen("tcp", "localhost:0")
			framework.ExpectNoError(err)
			defer listener.Close()

			serverAddr := listener.Addr().String()
			go func() {
				conn, _ := listener.Accept()
				if conn != nil {
					conn.Write([]byte("tunneled"))
					conn.Close()
				}
			}()

			// Create tunnel
			tunnel := network.NewSSHTunnel("localhost:0", serverAddr, logger)
			ctx := context.Background()
			err = tunnel.Start(ctx)
			framework.ExpectNoError(err)
			defer tunnel.Stop()

			time.Sleep(50 * time.Millisecond)

			// Connect through tunnel
			conn, err := net.Dial("tcp", tunnel.LocalAddr())
			if err == nil {
				defer conn.Close()
				buf := make([]byte, 8)
				conn.Read(buf)
				framework.ExpectEqual(string(buf), "tunneled")
			}
		})
	})

	ginkgo.Context("testing network map", ginkgo.Label("proxy"), func() {
		ginkgo.It("manages peers", func() {
			netmap := network.NewNetworkMap()

			netmap.AddPeer("peer1", "192.168.1.1:8080")
			netmap.AddPeer("peer2", "192.168.1.2:8080")

			gomega.Expect(netmap.Count()).To(gomega.Equal(2))

			peer, exists := netmap.GetPeer("peer1")
			framework.ExpectEqual(exists, true)
			framework.ExpectEqual(peer.ID, "peer1")

			peers := netmap.ListPeers()
			gomega.Expect(len(peers)).To(gomega.Equal(2))

			netmap.RemovePeer("peer1")
			gomega.Expect(netmap.Count()).To(gomega.Equal(1))
		})
	})

	ginkgo.Context("testing network client", ginkgo.Label("proxy"), func() {
		ginkgo.It("dials TCP connections", func() {
			// Create test server
			listener, err := net.Listen("tcp", "localhost:0")
			framework.ExpectNoError(err)
			defer listener.Close()

			go func() {
				conn, _ := listener.Accept()
				if conn != nil {
					conn.Close()
				}
			}()

			client := network.NewClient(listener.Addr().String())
			ctx := context.Background()
			conn, err := client.DialTCP(ctx)
			framework.ExpectNoError(err)
			if conn != nil {
				conn.Close()
			}
		})

		ginkgo.It("pings server", func() {
			// Create test server
			listener, err := net.Listen("tcp", "localhost:0")
			framework.ExpectNoError(err)
			defer listener.Close()

			go func() {
				conn, _ := listener.Accept()
				if conn != nil {
					conn.Close()
				}
			}()

			client := network.NewClient(listener.Addr().String())
			ctx := context.Background()
			err = client.Ping(ctx)
			framework.ExpectNoError(err)
		})
	})

	ginkgo.Context("testing network utilities", ginkgo.Label("proxy"), func() {
		ginkgo.It("parses host:port", func() {
			host, port, err := network.ParseHostPort("localhost:8080")
			framework.ExpectNoError(err)
			framework.ExpectEqual(host, "localhost")
			framework.ExpectEqual(port, 8080)
		})

		ginkgo.It("formats host:port", func() {
			addr := network.FormatHostPort("localhost", 8080)
			framework.ExpectEqual(addr, "localhost:8080")
		})

		ginkgo.It("detects localhost", func() {
			framework.ExpectEqual(network.IsLocalhost("localhost"), true)
			framework.ExpectEqual(network.IsLocalhost("127.0.0.1"), true)
			framework.ExpectEqual(network.IsLocalhost("192.168.1.1"), false)
		})

		ginkgo.It("finds free port", func() {
			port, err := network.GetFreePort()
			framework.ExpectNoError(err)
			gomega.Expect(port).To(gomega.BeNumerically(">", 0))
		})
	})
})
