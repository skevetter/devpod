package network

import (
	"sync"
	"time"

	"github.com/loft-sh/log"
)

// ConnectionInfo tracks information about a connection
type ConnectionInfo struct {
	ID         string
	RemoteAddr string
	StartTime  time.Time
	LastSeen   time.Time
}

// ConnectionTracker tracks active connections
type ConnectionTracker struct {
	mu          sync.RWMutex
	connections map[string]*ConnectionInfo
}

// NewConnectionTracker creates a new connection tracker
func NewConnectionTracker() *ConnectionTracker {
	return &ConnectionTracker{
		connections: make(map[string]*ConnectionInfo),
	}
}

// Add adds a connection to the tracker
func (ct *ConnectionTracker) Add(id, remoteAddr string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	now := time.Now()
	ct.connections[id] = &ConnectionInfo{
		ID:         id,
		RemoteAddr: remoteAddr,
		StartTime:  now,
		LastSeen:   now,
	}
}

// Remove removes a connection from the tracker
func (ct *ConnectionTracker) Remove(id string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	delete(ct.connections, id)
}

// Update updates the last seen time for a connection
func (ct *ConnectionTracker) Update(id string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	if conn, exists := ct.connections[id]; exists {
		conn.LastSeen = time.Now()
	}
}

// Get retrieves connection info
func (ct *ConnectionTracker) Get(id string) (*ConnectionInfo, bool) {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	conn, exists := ct.connections[id]
	return conn, exists
}

// List returns all active connections
func (ct *ConnectionTracker) List() []*ConnectionInfo {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	conns := make([]*ConnectionInfo, 0, len(ct.connections))
	for _, conn := range ct.connections {
		// Return a copy to avoid race conditions
		connCopy := *conn
		conns = append(conns, &connCopy)
	}
	return conns
}

// Count returns the number of active connections
func (ct *ConnectionTracker) Count() int {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return len(ct.connections)
}

// ConnTracker is a simple connection counter used by several services.
type ConnTracker struct {
	mu    sync.Mutex
	count int

	logger log.Logger
}

func (c *ConnTracker) Add(serviceName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.count++
	c.logger.Debugf("%s: added new connection, connection count %d\n", serviceName, c.count)
}

func (c *ConnTracker) Remove(serviceName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.count--
	c.logger.Debugf("%s: aemoved connection, connection count %d\n", serviceName, c.count)
}

func (c *ConnTracker) Count(serviceName string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.logger.Debugf("%s: get connection count %d\n", serviceName, c.count)
	return c.count
}
