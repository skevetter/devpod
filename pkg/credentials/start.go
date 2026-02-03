package credentials

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/pkg/agent/tunnel"
	devpodhttp "github.com/skevetter/devpod/pkg/http"
	portpkg "github.com/skevetter/devpod/pkg/port"
	"github.com/skevetter/devpod/pkg/random"
	"github.com/skevetter/log"
)

func StartCredentialsServer(ctx context.Context, client tunnel.TunnelClient, log log.Logger) (int, error) {
	port, err := portpkg.FindAvailablePort(random.InRange(13000, 17000))
	if err != nil {
		return 0, err
	}

	go func() {
		err := RunCredentialsServer(ctx, port, client, log)
		if err != nil {
			log.WithFields(logrus.Fields{"error": err}).Error("error running git credentials server")
		}
	}()

	if err := waitForServer(ctx, port, log); err != nil {
		return 0, err
	}
	return port, nil
}

func waitForServer(ctx context.Context, port int, log log.Logger) error {
	maxWait := time.Second * 4
	now := time.Now()
	for {
		err := PingURL(ctx, "http://localhost:"+strconv.Itoa(port))
		if err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
			}
		} else {
			log.Debug("credentials server started")
			return nil
		}

		if time.Since(now) > maxWait {
			log.Debug("credentials server did not start in time")
			return fmt.Errorf("credentials server did not start in time")
		}
	}
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
