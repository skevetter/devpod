package network

import (
	"net"
	"path/filepath"
)

// Dial returns a net.Conn to the network proxy socket.
func Dial() (net.Conn, error) {
	socketPath := filepath.Join(RootDir, NetworkProxySocket)
	return net.Dial("unix", socketPath)
}
