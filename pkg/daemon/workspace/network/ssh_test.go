package network

import (
	"testing"

	"github.com/loft-sh/log"
	"github.com/stretchr/testify/assert"
	"tailscale.com/tsnet"
)

func TestNewSSHService(t *testing.T) {
	tsServer := &tsnet.Server{}
	tracker := &ConnTracker{logger: log.Default}
	logger := log.Default

	service, err := NewSSHService(tsServer, tracker, logger)
	assert.NoError(t, err)
	assert.NotNil(t, service)
	assert.Equal(t, tsServer, service.tsServer)
	assert.Equal(t, tracker, service.tracker)
}
