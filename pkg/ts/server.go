package ts

import (
	"context"
	"net/url"

	"tailscale.com/envknob"
	"tailscale.com/ipn"
	"tailscale.com/ipn/store/mem"
	"tailscale.com/tsnet"
)

type ServerConfig struct {
	Hostname   string
	AuthKey    string
	Dir        string
	ControlURL *url.URL
	Ephemeral  bool
	Store      ipn.StateStore
	Logf       func(format string, args ...any)
}

// NewServer creates a configured tsnet.Server
func NewServer(cfg *ServerConfig) *tsnet.Server {
	if cfg.Store == nil && cfg.Logf != nil {
		cfg.Store, _ = mem.New(cfg.Logf, "")
	}

	envknob.Setenv("TS_DEBUG_TLS_DIAL_INSECURE_SKIP_VERIFY", "true")

	srv := &tsnet.Server{
		Hostname:  cfg.Hostname,
		AuthKey:   cfg.AuthKey,
		Dir:       cfg.Dir,
		Ephemeral: cfg.Ephemeral,
		Store:     cfg.Store,
		Logf:      cfg.Logf,
	}

	if cfg.ControlURL != nil {
		srv.ControlURL = cfg.ControlURL.String() + "/coordinator/"
	}

	return srv
}

// StartServer creates and starts a tsnet.Server
func StartServer(ctx context.Context, cfg *ServerConfig) (*tsnet.Server, error) {
	srv := NewServer(cfg)
	if _, err := srv.Up(ctx); err != nil {
		return nil, err
	}
	return srv, nil
}
