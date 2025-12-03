package network

import (
	"sync"
	"time"
)

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
