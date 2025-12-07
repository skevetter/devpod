package network

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/loft-sh/log"
	"github.com/skevetter/devpod/pkg/ts"
	"tailscale.com/client/tailscale"
	"tailscale.com/types/netmap"
)

const netMapCooldown = 5 * time.Second

// PeerInfo contains information about a network peer
type PeerInfo struct {
	ID       string
	Addr     string
	LastSeen time.Time
}

// NetworkMap tracks network peers
type NetworkMap struct {
	mu    sync.RWMutex
	peers map[string]*PeerInfo
}

// NewNetworkMap creates a new network map
func NewNetworkMap() *NetworkMap {
	return &NetworkMap{
		peers: make(map[string]*PeerInfo),
	}
}

// AddPeer adds a peer to the network map
func (nm *NetworkMap) AddPeer(id, addr string) {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	nm.peers[id] = &PeerInfo{
		ID:       id,
		Addr:     addr,
		LastSeen: time.Now(),
	}
}

// RemovePeer removes a peer from the network map
func (nm *NetworkMap) RemovePeer(id string) {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	delete(nm.peers, id)
}

// GetPeer retrieves peer information
func (nm *NetworkMap) GetPeer(id string) (*PeerInfo, bool) {
	nm.mu.RLock()
	defer nm.mu.RUnlock()
	peer, exists := nm.peers[id]
	return peer, exists
}

// ListPeers returns all peers
func (nm *NetworkMap) ListPeers() []*PeerInfo {
	nm.mu.RLock()
	defer nm.mu.RUnlock()
	peers := make([]*PeerInfo, 0, len(nm.peers))
	for _, peer := range nm.peers {
		peers = append(peers, peer)
	}
	return peers
}

// UpdatePeer updates the last seen time for a peer
func (nm *NetworkMap) UpdatePeer(id string) {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	if peer, exists := nm.peers[id]; exists {
		peer.LastSeen = time.Now()
	}
}

// Count returns the number of peers
func (nm *NetworkMap) Count() int {
	nm.mu.RLock()
	defer nm.mu.RUnlock()
	return len(nm.peers)
}

// NetmapWatcherService watches the Tailscale netmap and writes it to a file
type NetmapWatcherService struct {
	rootDir string
	lc      *tailscale.LocalClient
	log     log.Logger
}

// NewNetmapWatcherService creates a new NetmapWatcherService
func NewNetmapWatcherService(rootDir string, lc *tailscale.LocalClient, log log.Logger) *NetmapWatcherService {
	return &NetmapWatcherService{
		rootDir: rootDir,
		lc:      lc,
		log:     log,
	}
}

// Start begins watching the netmap
func (s *NetmapWatcherService) Start(ctx context.Context) {
	lastUpdate := time.Now()
	if err := ts.WatchNetmap(ctx, s.lc, func(netMap *netmap.NetworkMap) {
		if time.Since(lastUpdate) < netMapCooldown {
			return
		}
		lastUpdate = time.Now()
		nm, err := json.Marshal(netMap)
		if err != nil {
			s.log.Errorf("NetmapWatcherService: failed to marshal netmap: %v", err)
		} else {
			_ = os.WriteFile(filepath.Join(s.rootDir, "netmap.json"), nm, 0644)
		}
	}); err != nil {
		s.log.Errorf("NetmapWatcherService: failed to watch netmap: %v", err)
	}
}
