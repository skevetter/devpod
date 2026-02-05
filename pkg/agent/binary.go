package agent

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	devpodhttp "github.com/skevetter/devpod/pkg/http"
	"github.com/skevetter/log"
)

type RetryConfig struct {
	MaxAttempts  int
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Deadline     time.Time
}

type RetryFunc func(attempt int) error

func RetryWithDeadline(
	ctx context.Context,
	log log.Logger,
	cfg RetryConfig,
	fn RetryFunc,
) error {
	cfg.applyDefaults()
	delay := cfg.InitialDelay

	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		if err := cfg.checkPreConditions(ctx, attempt-1); err != nil {
			return err
		}

		err := fn(attempt)
		if err == nil {
			return nil
		}

		if attempt == cfg.MaxAttempts {
			return fmt.Errorf("agent injection failed after %d attempts: %w", attempt, err)
		}

		delay = cfg.handleRetry(&retryContext{
			ctx:     ctx,
			log:     log,
			attempt: attempt,
			err:     err,
			delay:   delay,
		})
		if delay == 0 {
			return ctx.Err()
		}
	}

	return fmt.Errorf("retry loop exited unexpectedly")
}

func (cfg *RetryConfig) checkPreConditions(ctx context.Context, attemptsCompleted int) error {
	if err := cfg.checkDeadline(attemptsCompleted); err != nil {
		return err
	}
	return checkContextCancelled(ctx)
}

type retryContext struct {
	ctx     context.Context
	log     log.Logger
	attempt int
	err     error
	delay   time.Duration
}

func (cfg *RetryConfig) handleRetry(rctx *retryContext) time.Duration {
	sleep := calculateSleep(rctx.delay, cfg)

	rctx.log.Debugf("retrying attempt %d after %v: %v", rctx.attempt, sleep, rctx.err)

	if err := sleepWithContext(rctx.ctx, sleep); err != nil {
		return 0
	}

	newDelay := rctx.delay * 2
	return min(newDelay, cfg.MaxDelay)
}

func (cfg *RetryConfig) applyDefaults() {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 1
	}
	if cfg.InitialDelay <= 0 {
		cfg.InitialDelay = time.Second
	}
	if cfg.MaxDelay <= 0 {
		cfg.MaxDelay = 30 * time.Second
	}
}

func (cfg *RetryConfig) checkDeadline(attemptsCompleted int) error {
	if cfg.Deadline.IsZero() || !time.Now().After(cfg.Deadline) {
		return nil
	}
	return fmt.Errorf("%w after %d attempts", ErrInjectTimeout, attemptsCompleted)
}

func checkContextCancelled(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func calculateSleep(delay time.Duration, cfg *RetryConfig) time.Duration {
	sleep := delay
	if !cfg.Deadline.IsZero() {
		remaining := time.Until(cfg.Deadline)
		if remaining > 0 && sleep > remaining {
			sleep = remaining
		}
	}
	return sleep
}

func sleepWithContext(ctx context.Context, duration time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(duration):
		return nil
	}
}

type BinarySource interface {
	GetBinary(ctx context.Context, arch string) (io.ReadCloser, error)
	SourceName() string
}

type BinaryManager struct {
	sources []BinarySource
	logger  log.Logger
}

func NewBinaryManager(logger log.Logger, downloadURL string) *BinaryManager {
	cachePath := filepath.Join(os.TempDir(), "devpod-cache")
	cache := &BinaryCache{BaseDir: cachePath}

	return &BinaryManager{
		sources: []BinarySource{
			&InjectSource{},
			&FileCacheSource{Cache: cache},
			&HTTPDownloadSource{BaseURL: downloadURL, Cache: cache},
		},
		logger: logger,
	}
}

func (m *BinaryManager) AcquireBinary(ctx context.Context, arch string) (io.ReadCloser, string, error) {
	for _, source := range m.sources {
		binary, err := source.GetBinary(ctx, arch)
		if err == nil {
			m.logger.Debugf("acquired binary from %s", source.SourceName())
			return binary, source.SourceName(), nil
		}
		m.logger.Debugf("source %s failed: %v", source.SourceName(), err)
	}
	return nil, "", ErrBinaryNotFound
}

type BinaryCache struct {
	BaseDir string
}

func (c *BinaryCache) Get(arch string) (io.ReadCloser, error) {
	return os.Open(c.pathFor(arch))
}

func (c *BinaryCache) Set(arch string, data io.Reader) error {
	return c.atomicWrite(c.pathFor(arch), data)
}

func (c *BinaryCache) pathFor(arch string) string {
	return filepath.Join(c.BaseDir, "devpod-"+osLinux+"-"+arch)
}

func (c *BinaryCache) atomicWrite(path string, data io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil { // #nosec G301
		return err
	}

	file, err := os.CreateTemp(filepath.Dir(path), "devpod-*.tmp")
	if err != nil {
		return err
	}
	temp := file.Name()

	if _, err := io.Copy(file, data); err != nil {
		_ = file.Close()
		_ = os.Remove(temp)
		return err
	}

	if err := file.Chmod(0755); err != nil {
		_ = file.Close()
		_ = os.Remove(temp)
		return err
	}

	if err := file.Close(); err != nil {
		_ = os.Remove(temp)
		return err
	}

	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(temp)
		return err
	}
	return nil
}

type InjectSource struct{}

func (s *InjectSource) GetBinary(ctx context.Context, arch string) (io.ReadCloser, error) {
	if !s.matchesCurrentRuntime(arch) {
		return nil, ErrArchMismatch
	}
	return s.openCurrentExecutable()
}

func (s *InjectSource) SourceName() string {
	return "local executable"
}

func (s *InjectSource) matchesCurrentRuntime(arch string) bool {
	return runtime.GOOS == osLinux && runtime.GOARCH == arch
}

func (s *InjectSource) openCurrentExecutable() (io.ReadCloser, error) {
	path, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return os.Open(path) // #nosec G304
}

type FileCacheSource struct {
	Cache *BinaryCache
}

func (s *FileCacheSource) GetBinary(ctx context.Context, arch string) (io.ReadCloser, error) {
	return s.Cache.Get(arch)
}

func (s *FileCacheSource) SourceName() string {
	return "local cache"
}

type HTTPDownloadSource struct {
	BaseURL string
	Cache   *BinaryCache
}

func (s *HTTPDownloadSource) GetBinary(ctx context.Context, arch string) (io.ReadCloser, error) {
	downloadURL, err := s.buildDownloadURL(arch)
	if err != nil {
		return nil, err
	}

	resp, err := s.downloadFile(ctx, downloadURL)
	if err != nil {
		return nil, err
	}

	if s.Cache != nil {
		return s.cacheAndReturn(arch, resp.Body)
	}

	return resp.Body, nil
}

func (s *HTTPDownloadSource) SourceName() string {
	return "http download"
}

func (s *HTTPDownloadSource) buildDownloadURL(arch string) (string, error) {
	binaryName := "devpod-" + osLinux + "-" + arch
	downloadURL, err := url.JoinPath(s.BaseURL, binaryName)
	if err != nil {
		return "", fmt.Errorf("failed to construct download URL: %w", err)
	}
	return downloadURL, nil
}

func (s *HTTPDownloadSource) downloadFile(ctx context.Context, downloadURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := devpodhttp.GetHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download binary: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("bad status: %s", resp.Status)
	}

	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/html") {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("received HTML instead of binary from %s (check if the download URL is correct)", downloadURL)
	}

	return resp, nil
}

// cacheAndReturn streams the binary to the caller while simultaneously caching it.
// The caller MUST fully read or close the returned reader to avoid goroutine leaks.
func (s *HTTPDownloadSource) cacheAndReturn(arch string, body io.ReadCloser) (io.ReadCloser, error) {
	pr, pw := io.Pipe()

	go func() {
		var streamErr error
		defer func() {
			_ = body.Close()
			if streamErr != nil {
				_ = pw.CloseWithError(streamErr)
			} else {
				_ = pw.Close()
			}
		}()

		if !s.prepareCacheDir(arch, body, pw, &streamErr) {
			return
		}

		s.streamAndCache(arch, body, pw, &streamErr)
	}()

	return pr, nil
}

func (s *HTTPDownloadSource) prepareCacheDir(
	arch string,
	body io.ReadCloser,
	pw *io.PipeWriter,
	streamErr *error,
) bool {
	cachePath := s.Cache.pathFor(arch)
	if err := os.MkdirAll(filepath.Dir(cachePath), 0750); err != nil { // #nosec G301
		// Cache directory creation failed; fall back to direct streaming
		if _, copyErr := io.Copy(pw, body); copyErr != nil {
			*streamErr = fmt.Errorf("mkdir failed (%v), fallback copy failed: %w", err, copyErr)
		}
		return false
	}
	return true
}

func (s *HTTPDownloadSource) streamAndCache(arch string, body io.ReadCloser, pw *io.PipeWriter, streamErr *error) {
	cachePath := s.Cache.pathFor(arch)
	file, tmpPath, err := s.createTempFile(cachePath, body, pw, streamErr)
	if err != nil {
		return
	}

	success := false
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
		if !success {
			_ = os.Remove(tmpPath)
		}
	}()

	if !s.writeToFile(file, body, pw, streamErr) {
		return
	}

	closeErr := file.Close()
	closed = true
	if closeErr != nil {
		*streamErr = closeErr
		return
	}

	if err := os.Rename(tmpPath, cachePath); err == nil {
		success = true
	}
}

func (s *HTTPDownloadSource) createTempFile(
	cachePath string,
	body io.ReadCloser,
	pw *io.PipeWriter,
	streamErr *error,
) (*os.File, string, error) {
	file, err := os.CreateTemp(filepath.Dir(cachePath), "devpod-agent-*.tmp")
	if err != nil {
		if _, copyErr := io.Copy(pw, body); copyErr != nil {
			*streamErr = copyErr
		}
		return nil, "", err
	}
	return file, file.Name(), nil
}

func (s *HTTPDownloadSource) writeToFile(file *os.File, body io.ReadCloser, pw *io.PipeWriter, streamErr *error) bool {
	mw := io.MultiWriter(file, pw)
	if _, err := io.Copy(mw, body); err != nil {
		*streamErr = err
		return false
	}

	if err := file.Chmod(0755); err != nil {
		*streamErr = err
		return false
	}

	if err := file.Sync(); err != nil {
		*streamErr = err
		return false
	}

	return true
}
