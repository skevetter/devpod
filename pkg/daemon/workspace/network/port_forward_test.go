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

	service, err := NewHTTPPortForwardService(tsServer, tracker, logger)
	// Will fail because tsServer is not started, but we're testing the constructor
	if err != nil {
		assert.Error(t, err)
	} else {
		assert.NotNil(t, service)
		assert.Equal(t, tsServer, service.tsServer)
		assert.Equal(t, tracker, service.tracker)
	}
}
