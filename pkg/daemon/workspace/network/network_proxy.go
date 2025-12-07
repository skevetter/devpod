package network

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/loft-sh/log"
	"github.com/mwitkow/grpc-proxy/proxy"
	"github.com/soheilhy/cmux"
	"google.golang.org/grpc"
	"tailscale.com/client/tailscale"
	"tailscale.com/tsnet"
)

const (
	HeaderTargetHost      = "x-target-host"
	HeaderTargetPort      = "x-target-port"
	HeaderProxyPort       = "x-proxy-port"
	networkProxyLogPrefix = "NetworkProxyService: "
)

// NetworkProxyService proxies gRPC and HTTP over DevPod network
type NetworkProxyService struct {
	mainListener net.Listener
	grpcServer   *grpc.Server
	httpServer   *http.Server
	tsServer     *tsnet.Server
	log          log.Logger
	socketPath   string
	mux          cmux.CMux
	grpcDirector *GrpcDirector
	httpProxy    *HttpProxyHandler
}

// NewNetworkProxyService creates service listening on Unix socket
func NewNetworkProxyService(socketPath string, tsServer *tsnet.Server, lc *tailscale.LocalClient, config *ServerConfig, projectName, workspaceName string, log log.Logger) (*NetworkProxyService, error) {
	_ = os.Remove(socketPath)
	if err := os.MkdirAll("/var/run/devpod", 0755); err != nil {
		return nil, fmt.Errorf("create socket dir: %w", err)
	}

	l, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on socket %s %w", socketPath, err)
	}

	if err := os.Chmod(socketPath, 0777); err != nil {
		_ = l.Close()
		return nil, fmt.Errorf("failed to set socket permissions on %s %w", socketPath, err)
	}

	log.Infof(networkProxyLogPrefix+"network proxy listening on socket %s", socketPath)

	grpcDirector := NewGrpcDirector(tsServer, log)
	httpProxy := NewHttpProxyHandler(tsServer, lc, config, projectName, workspaceName, log)

	return &NetworkProxyService{
		mainListener: l,
		tsServer:     tsServer,
		log:          log,
		socketPath:   socketPath,
		grpcDirector: grpcDirector,
		httpProxy:    httpProxy,
	}, nil
}

// Start runs the proxy server
func (s *NetworkProxyService) Start(ctx context.Context) error {
	s.mux = cmux.New(s.mainListener)

	grpcL := s.mux.MatchWithWriters(cmux.HTTP2MatchHeaderFieldSendSettings("content-type", "application/grpc"))
	httpL := s.mux.Match(cmux.Any())

	s.grpcServer = grpc.NewServer(
		grpc.UnknownServiceHandler(proxy.TransparentHandler(s.grpcDirector.DirectorFunc)),
	)
	s.httpServer = &http.Server{
		Handler: s.httpProxy,
	}

	var runWg sync.WaitGroup
	errChan := make(chan error, 3)

	runWg.Go(func() {
		s.log.Debugf(networkProxyLogPrefix + "starting gRPC server")
		if err := s.grpcServer.Serve(grpcL); err != nil && !errors.Is(err, grpc.ErrServerStopped) && !errors.Is(err, cmux.ErrListenerClosed) {
			s.log.Errorf(networkProxyLogPrefix+"gRPC server error %v", err)
			errChan <- fmt.Errorf("gRPC server error %w", err)
		} else {
			s.log.Debugf(networkProxyLogPrefix + "gRPC server stopped")
		}
	})

	runWg.Go(func() {
		s.log.Debugf(networkProxyLogPrefix + "starting HTTP server")
		if err := s.httpServer.Serve(httpL); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, cmux.ErrListenerClosed) {
			s.log.Errorf(networkProxyLogPrefix+"HTTP server error %v", err)
			errChan <- fmt.Errorf("HTTP server error %w", err)
		} else {
			s.log.Debugf(networkProxyLogPrefix + "HTTP server stopped")
		}
	})

	runWg.Go(func() {
		s.log.Infof(networkProxyLogPrefix + "starting server")
		err := s.mux.Serve()
		if err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, cmux.ErrListenerClosed) {
			s.log.Errorf(networkProxyLogPrefix+"server error %v", err)
			errChan <- fmt.Errorf("server error %w", err)
		} else {
			s.log.Infof(networkProxyLogPrefix + "server stopped")
		}
	})

	s.log.Infof(networkProxyLogPrefix+"successfully started listeners on %s", s.socketPath)

	var finalErr error
	select {
	case <-ctx.Done():
		s.log.Infof(networkProxyLogPrefix + "context cancelled, shutting down proxy service")
		finalErr = ctx.Err()
	case err := <-errChan:
		s.log.Errorf(networkProxyLogPrefix+"server error triggered shutdown %v", err)
		finalErr = err
	}

	s.Stop()

	s.log.Debugf(networkProxyLogPrefix + "waiting for servers to exit")
	runWg.Wait()
	s.log.Debugf(networkProxyLogPrefix + "all servers exited")

	return finalErr
}

func (s *NetworkProxyService) Stop() {
	s.log.Infof(networkProxyLogPrefix + "stopping proxy service")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var shutdownWg sync.WaitGroup
	shutdownWg.Add(2)

	go func() {
		defer shutdownWg.Done()
		if s.grpcServer != nil {
			s.grpcServer.GracefulStop()
			s.log.Debugf(networkProxyLogPrefix + "gRPC server stopped")
		}
	}()

	go func() {
		defer shutdownWg.Done()
		if s.httpServer != nil {
			if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
				s.log.Warnf(networkProxyLogPrefix+"HTTP server shutdown error %v", err)
			} else {
				s.log.Debugf(networkProxyLogPrefix + "HTTP server stopped")
			}
		}
	}()

	s.log.Infof(networkProxyLogPrefix + "waiting for servers to stop")

	waitDone := make(chan struct{})
	go func() {
		defer close(waitDone)
		shutdownWg.Wait()
	}()

	select {
	case <-waitDone:
		s.log.Debugf(networkProxyLogPrefix + "all server shutdowns completed")
	case <-shutdownCtx.Done():
		s.log.Warnf(networkProxyLogPrefix + "graceful shutdown timed out after waiting for servers")
	}

	s.log.Debugf(networkProxyLogPrefix + "listener and socket cleanup")

	if s.mainListener != nil {
		s.log.Debugf(networkProxyLogPrefix + "closing main listener")
		if err := s.mainListener.Close(); err != nil {
			if !errors.Is(err, net.ErrClosed) && !errors.Is(err, cmux.ErrListenerClosed) {
				s.log.Errorf(networkProxyLogPrefix+"error closing main listener %v", err)
			} else {
				s.log.Debugf(networkProxyLogPrefix + "main listener closed")
			}
		} else {
			s.log.Debugf(networkProxyLogPrefix + "main listener closed successfully")
		}
	} else {
		s.log.Warnf(networkProxyLogPrefix + "main listener was nil during stop")
	}

	s.log.Debugf(networkProxyLogPrefix+"removing socket file %s", s.socketPath)
	if err := os.Remove(s.socketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		s.log.Warnf(networkProxyLogPrefix+"failed to remove socket file %s %v", s.socketPath, err)
	} else if err == nil {
		s.log.Debugf(networkProxyLogPrefix+"removed socket file %s", s.socketPath)
	}

	s.log.Infof(networkProxyLogPrefix + "proxy service stopped")
}
