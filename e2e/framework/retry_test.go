package framework

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsRetryableDockerError_RateLimit(t *testing.T) {
	stderr := `GET https://index.docker.io/v2/library/ubuntu/manifests/latest: ` +
		`TOOMANYREQUESTS: You have reached your unauthenticated pull rate limit.`
	assert.True(t, isRetryableDockerError(stderr))
}

func TestIsRetryableDockerError_Timeout(t *testing.T) {
	stderr := `Get "https://registry-1.docker.io/v2/": net/http: TLS handshake timeout`
	assert.True(t, isRetryableDockerError(stderr))
}

func TestIsRetryableDockerError_IOTimeout(t *testing.T) {
	stderr := `Get "https://registry-1.docker.io/v2/library/ubuntu/manifests/latest": i/o timeout`
	assert.True(t, isRetryableDockerError(stderr))
}

func TestIsRetryableDockerError_ConnectionReset(t *testing.T) {
	stderr := `error pulling image: read tcp 10.0.0.1:443: read: connection reset by peer`
	assert.True(t, isRetryableDockerError(stderr))
}

func TestIsRetryableDockerError_ServiceUnavailable(t *testing.T) {
	stderr := `received unexpected HTTP status: 503 Service Unavailable`
	assert.True(t, isRetryableDockerError(stderr))
}

func TestIsRetryableDockerError_RealFailure(t *testing.T) {
	stderr := `error resolving dockerfile: dockerfile not found`
	assert.False(t, isRetryableDockerError(stderr))
}

func TestIsRetryableDockerError_Empty(t *testing.T) {
	assert.False(t, isRetryableDockerError(""))
}
