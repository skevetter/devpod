package network

const (
	// TSPortForwardPort is the fixed port on which the workspace HTTP reverse proxy listens
	TSPortForwardPort = "12051"

	// NetworkProxySocket is the socket for network proxy
	NetworkProxySocket = "devpod-net.sock"

	// RootDir is the default root directory
	RootDir = "/var/run/devpod"
)
