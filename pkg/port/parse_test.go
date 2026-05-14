package port

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePortSpec_ServiceNameTarget(t *testing.T) {
	mapping, err := ParsePortSpec("8080:nginx:80")
	require.NoError(t, err)
	assert.Equal(t, "tcp", mapping.Host.Protocol)
	assert.Equal(t, "localhost:8080", mapping.Host.Address)
	assert.Equal(t, "tcp", mapping.Container.Protocol)
	assert.Equal(t, "nginx:80", mapping.Container.Address)
}

func TestParsePortSpec_ServiceNameTargetWithLocalBindHost(t *testing.T) {
	mapping, err := ParsePortSpec("127.0.0.1:8080:nginx:80")
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1:8080", mapping.Host.Address)
	assert.Equal(t, "nginx:80", mapping.Container.Address)
}

func TestParsePortSpec_PreservesLocalBindDisambiguation(t *testing.T) {
	mapping, err := ParsePortSpec("localhost:8080:80")
	require.NoError(t, err)
	assert.Equal(t, "localhost:8080", mapping.Host.Address)
	assert.Equal(t, "localhost:80", mapping.Container.Address)
}

func TestParsePortSpec_AllowsTargetIPHosts(t *testing.T) {
	mapping, err := ParsePortSpec("8080:10.0.0.2:80")
	require.NoError(t, err)
	assert.Equal(t, "localhost:8080", mapping.Host.Address)
	assert.Equal(t, "10.0.0.2:80", mapping.Container.Address)
}

func TestParsePortSpec_RejectsNonIPListenHost(t *testing.T) {
	_, err := ParsePortSpec("app:8080:nginx:80")
	require.Error(t, err)
	assert.ErrorContains(t, err, "not an ip address app")
}

func TestParsePortSpec_RejectsEmptyTargetHost(t *testing.T) {
	_, err := ParsePortSpec("8080::80")
	require.Error(t, err)
	assert.ErrorContains(t, err, "target host is empty")
}

func TestParsePortSpec_RejectsEmptyListenHost(t *testing.T) {
	_, err := ParsePortSpec(":8080:nginx:80")
	require.Error(t, err)
	assert.ErrorContains(t, err, "listen host is empty")
}

func TestParsePortSpec_DelegatesUnixSocketMappings(t *testing.T) {
	mapping, err := ParsePortSpec("/tmp/local.sock:/tmp/remote.sock")
	require.NoError(t, err)
	assert.Equal(t, Mapping{
		Host:      Address{Protocol: "unix", Address: "/tmp/local.sock"},
		Container: Address{Protocol: "unix", Address: "/tmp/remote.sock"},
	}, mapping)
}

func TestParsePortSpec_RejectsEmptyListenUnixSocketPath(t *testing.T) {
	_, err := ParsePortSpec(":/tmp/remote.sock")
	require.Error(t, err)
	assert.ErrorContains(t, err, "listen host is empty")
}

func TestParsePortSpec_RejectsEmptyTargetUnixSocketPath(t *testing.T) {
	_, err := ParsePortSpec("/tmp/local.sock:")
	require.Error(t, err)
	assert.ErrorContains(t, err, "target host is empty")
}
