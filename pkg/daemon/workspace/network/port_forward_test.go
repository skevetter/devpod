package network

import (
	"testing"

	"github.com/loft-sh/log"
	"github.com/stretchr/testify/assert"
	"tailscale.com/tsnet"
)

func TestNewHTTPPortForwardService(t *testing.T) {
	tsServer := &tsnet.Server{}
	tracker := &ConnTracker{logger: log.Default}
	logger := log.Default

	service, _ := NewHTTPPortForwardService(tsServer, tracker, logger)
	assert.NotNil(t, service)
	assert.Equal(t, tsServer, service.tsServer)
	assert.Equal(t, tracker, service.tracker)
}
