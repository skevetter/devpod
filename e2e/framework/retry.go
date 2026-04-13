package framework

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	"k8s.io/apimachinery/pkg/util/wait"
)

// dockerPullBackoff defines retry timing for transient Docker registry errors.
// 4 total attempts (1 initial + 3 retries) with waits of ~30s, ~60s, ~120s.
var dockerPullBackoff = wait.Backoff{
	Steps:    4,
	Duration: 30 * time.Second,
	Factor:   2.0,
	Jitter:   0.1,
}

// retryableDockerPatterns are stderr substrings indicating a transient Docker
// registry error that is worth retrying.
var retryableDockerPatterns = []string{
	"TOOMANYREQUESTS",
	"rate limit",
	"TLS handshake timeout",
	"i/o timeout",
	"connection reset by peer",
	"503 Service Unavailable",
}

// isRetryableDockerError returns true if stderr contains a transient Docker
// registry error (rate limits, timeouts, connection resets).
func isRetryableDockerError(stderr string) bool {
	lower := strings.ToLower(stderr)
	for _, pattern := range retryableDockerPatterns {
		if strings.Contains(lower, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}

// execWithDockerRetry runs fn and retries if stderr indicates a transient
// Docker registry error. Returns the last stdout, stderr, and error.
func execWithDockerRetry(
	ctx context.Context,
	fn func(ctx context.Context) (stdout, stderr string, err error),
) (string, string, error) {
	var lastStdout, lastStderr string
	var lastErr error
	attempt := 0

	err := wait.ExponentialBackoffWithContext(ctx, dockerPullBackoff,
		func(ctx context.Context) (bool, error) {
			attempt++
			lastStdout, lastStderr, lastErr = fn(ctx)
			if lastErr == nil {
				return true, nil // success
			}
			if isRetryableDockerError(lastStderr) {
				ginkgo.GinkgoWriter.Printf(
					"[retry] attempt %d failed with transient Docker error, retrying: %s\n",
					attempt, lastErr,
				)
				return false, nil // retry
			}
			return false, lastErr // non-retryable, stop immediately
		},
	)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return lastStdout, lastStderr, err
	}
	if err != nil && lastErr != nil {
		return lastStdout, lastStderr, fmt.Errorf("after %d attempts: %w", attempt, lastErr)
	}
	return lastStdout, lastStderr, err
}
