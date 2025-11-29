package credentials

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/loft-sh/log"
	"github.com/skevetter/devpod/pkg/agent/tunnel"
	devpodhttp "github.com/skevetter/devpod/pkg/http"
	portpkg "github.com/skevetter/devpod/pkg/port"
	"github.com/skevetter/devpod/pkg/random"
)

func StartCredentialsServer(ctx context.Context, cancel context.CancelFunc, client tunnel.TunnelClient, log log.Logger) (int, error) {
	port, err := portpkg.FindAvailablePort(random.InRange(13000, 17000))
	if err != nil {
		return 0, err
	}

	go func() {
		defer cancel()

		err := RunCredentialsServer(ctx, port, client, log)
		if err != nil {
			log.Errorf("Error running git credentials server: %v", err)
		}
	}()

	// wait until credentials server is up
	maxWait := time.Second * 4
	now := time.Now()
Outer:
	for {
		err := PingURL(ctx, "http://localhost:"+strconv.Itoa(port))
		if err != nil {
			select {
			case <-ctx.Done():
				break Outer
			case <-time.After(time.Second):
			}
		} else {
			log.Debugf("Credentials server started...")
			break
		}

		if time.Since(now) > maxWait {
			log.Debugf("Credentials server didn't start in time...")
			break
		}
	}

	return port, nil
}

func PingURL(ctx context.Context, url string) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(timeoutCtx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := devpodhttp.GetHTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return nil
}
