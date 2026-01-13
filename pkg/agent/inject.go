package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	devpodhttp "github.com/skevetter/devpod/pkg/http"
	"github.com/skevetter/devpod/pkg/inject"
	"github.com/skevetter/devpod/pkg/shell"
	"github.com/skevetter/devpod/pkg/version"
	"github.com/skevetter/log"
)

var (
	ErrBinaryNotFound = errors.New("agent binary not found")
	ErrInjectTimeout  = errors.New("injection timeout")
	ErrArchMismatch   = errors.New("architecture mismatch")
)

var waitForInstanceConnectionTimeout = time.Minute * 5

// InjectOptions defines the parameters for injecting the DevPod agent into a remote environment.
type InjectOptions struct {
	// Ctx is the context for the injection operation. Required.
	Ctx context.Context
	// Exec is the function used to execute commands on the remote machine. Required.
	Exec inject.ExecFunc
	// Log is the logger for capturing injection output. Required.
	Log log.Logger

	// IsLocal indicates if the injection target is the local machine.
	IsLocal bool
	// RemoteAgentPath is the path where the agent binary should be placed on the remote machine. Defaults to RemoteDevPodHelperLocation.
	RemoteAgentPath string
	// DownloadURL is the base URL to download the agent binary from. Defaults to DefaultAgentDownloadURL().
	DownloadURL string
	// PreferDownload forces downloading the agent even if a local binary is available.
	// Defaults to true for release versions, false for dev versions.
	PreferDownload *bool
	// Timeout is the maximum duration to wait for the injection to complete. Defaults to 5 minutes.
	Timeout time.Duration

	// Command is the command to execute after successful injection.
	Command string

	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	// LocalVersion is the version of the local DevPod binary. Defaults to version.GetVersion().
	LocalVersion string
	// RemoteVersion is the expected version of the remote agent. Defaults to LocalVersion.
	RemoteVersion string
	// SkipVersionCheck disables the validation of the remote agent's version. Defaults to false, unless DEVPOD_AGENT_URL is set.
	SkipVersionCheck bool
	// MetricsCollector handles the recording of injection metrics. Defaults to LogMetricsCollector.
	MetricsCollector MetricsCollector
}

func (o *InjectOptions) ApplyDefaults() {
	if o.RemoteAgentPath == "" {
		o.RemoteAgentPath = RemoteDevPodHelperLocation
	}
	if o.DownloadURL == "" {
		o.DownloadURL = DefaultAgentDownloadURL()
	}
	if o.Timeout == 0 {
		o.Timeout = waitForInstanceConnectionTimeout
	}
	if o.LocalVersion == "" {
		o.LocalVersion = version.GetVersion()
	}
	if o.RemoteVersion == "" {
		o.RemoteVersion = o.LocalVersion
	}

	hasCustomAgentURL := os.Getenv(EnvDevPodAgentURL) != ""
	if hasCustomAgentURL {
		o.SkipVersionCheck = true
	}

	if o.PreferDownload == nil {
		if hasCustomAgentURL {
			o.PreferDownload = Bool(true)
		} else {
			if version.GetVersion() == version.DevVersion {
				o.PreferDownload = Bool(false)
			} else {
				o.PreferDownload = Bool(true)
			}
		}
	}
}

func Bool(b bool) *bool {
	return &b
}

func (o *InjectOptions) Validate() error {
	if o.Ctx == nil {
		return fmt.Errorf("context is required")
	}
	if o.Exec == nil {
		return fmt.Errorf("exec function is required")
	}
	if o.Log == nil {
		return fmt.Errorf("logger is required")
	}
	return nil
}

func InjectAgent(opts *InjectOptions) error {
	opts.ApplyDefaults()
	if err := opts.Validate(); err != nil {
		return err
	}

	if opts.MetricsCollector == nil {
		opts.MetricsCollector = &LogMetricsCollector{Log: opts.Log}
	}
	metrics := &InjectionMetrics{StartTime: time.Now(), BinarySource: "existing"}
	defer func() {
		metrics.EndTime = time.Now()
		opts.MetricsCollector.RecordInjection(metrics)
	}()

	if opts.IsLocal {
		return injectLocally(opts)
	}

	opts.Log.WithFields(logrus.Fields{
		"localVersion":  opts.LocalVersion,
		"remoteVersion": opts.RemoteVersion,
		"skipCheck":     opts.SkipVersionCheck,
	}).Debug("starting agent injection")

	vc := newVersionChecker(opts)
	bm := NewBinaryManager(opts.Log, opts.DownloadURL)
	retry := DefaultRetryStrategy()
	if opts.Timeout > 0 {
		retry.Timeout = opts.Timeout
	}

	return retry.WithRetry(opts.Ctx, opts.Log, func(attempt int) error {
		return injectAgent(attempt, opts, bm, vc, metrics)
	})
}

func injectLocally(opts *InjectOptions) error {
	if opts.Command == "" {
		return nil
	}
	opts.Log.Debug("execute command locally")
	return shell.RunEmulatedShell(opts.Ctx, opts.Command, opts.Stdin, opts.Stdout, opts.Stderr, nil)
}

func injectAgent(
	attempt int,
	opts *InjectOptions,
	bm *BinaryManager,
	vc *versionChecker,
	metrics *InjectionMetrics,
) error {
	metrics.Attempts = attempt

	buf := &bytes.Buffer{}
	var stderr io.Writer = buf
	if opts.Stderr != nil {
		stderr = io.MultiWriter(opts.Stderr, buf)
	}

	binaryLoader := func(arm bool) (io.ReadCloser, error) {
		arch := "amd64"
		if arm {
			arch = "arm64"
		}
		stream, source, err := bm.AcquireBinary(opts.Ctx, arch)
		if err != nil {
			return nil, err
		}
		metrics.BinarySource = source
		return stream, nil
	}

	scriptParams := &inject.Params{
		Command:             opts.Command,
		AgentRemotePath:     opts.RemoteAgentPath,
		DownloadURLs:        inject.NewDownloadURLs(opts.DownloadURL),
		ExistsCheck:         vc.buildExistsCheck(opts.RemoteAgentPath),
		PreferAgentDownload: *opts.PreferDownload,
		ShouldChmodPath:     true,
	}

	wasExecuted, err := inject.Inject(inject.InjectOptions{
		Ctx:          opts.Ctx,
		Exec:         opts.Exec,
		LocalFile:    binaryLoader,
		ScriptParams: scriptParams,
		Stdin:        opts.Stdin,
		Stdout:       opts.Stdout,
		Stderr:       stderr,
		Timeout:      opts.Timeout,
		Log:          opts.Log,
	})

	if err != nil {
		metrics.Error = err
		if wasExecuted {
			return &InjectError{
				Stage: "command_exec",
				Cause: fmt.Errorf("%w: %s", err, buf.String()),
			}
		}
		return &InjectError{Stage: "inject", Cause: err}
	}

	if !opts.SkipVersionCheck {
		detectedVersion, err := vc.validateRemoteAgent(opts.Ctx, opts.Exec, opts.RemoteAgentPath, opts.Log)
		if detectedVersion != "" {
			metrics.AgentVersion = detectedVersion
		}
		if err != nil {
			opts.Log.WithFields(logrus.Fields{"error": err}).Warn("Version validation failed")
			metrics.VersionCheck = false
		} else {
			metrics.VersionCheck = true
		}
	} else {
		metrics.AgentVersion = opts.RemoteVersion
	}

	metrics.Success = true
	return nil
}

type InjectError struct {
	Stage string
	Cause error
}

func (e *InjectError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %v", e.Stage, e.Cause)
	}
	return fmt.Sprintf("[%s] unknown error", e.Stage)
}

func (e *InjectError) Unwrap() error {
	return e.Cause
}

type InjectionMetrics struct {
	StartTime    time.Time
	EndTime      time.Time
	Attempts     int
	BinarySource string
	AgentVersion string
	VersionCheck bool
	Success      bool
	Error        error
}

type MetricsCollector interface {
	RecordInjection(metrics *InjectionMetrics)
}

type LogMetricsCollector struct {
	Log log.Logger
}

func (c *LogMetricsCollector) RecordInjection(metrics *InjectionMetrics) {
	c.Log.WithFields(logrus.Fields{
		"duration":     metrics.EndTime.Sub(metrics.StartTime),
		"attempts":     metrics.Attempts,
		"binarySource": metrics.BinarySource,
		"agentVersion": metrics.AgentVersion,
		"versionCheck": metrics.VersionCheck,
		"success":      metrics.Success,
	}).Debug("agent injection metrics")
}

type RetryStrategy struct {
	MaxAttempts  int
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Timeout      time.Duration
}

func DefaultRetryStrategy() *RetryStrategy {
	return &RetryStrategy{
		MaxAttempts:  30,
		InitialDelay: 3 * time.Second,
		MaxDelay:     15 * time.Second,
		Timeout:      5 * time.Minute,
	}
}

type RetryableFunc func(attempt int) error

func (s *RetryStrategy) WithRetry(ctx context.Context, log log.Logger, fn RetryableFunc) error {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	delay := s.InitialDelay
	attempt := 1

	for {
		err := fn(attempt)
		if err == nil {
			return nil
		}

		if attempt >= s.MaxAttempts {
			return err
		}

		select {
		case <-ctx.Done():
			return ErrInjectTimeout
		case <-time.After(delay):
			delay *= 2
			if delay > s.MaxDelay {
				delay = s.MaxDelay
			}
			attempt++
		}
	}
}

type versionChecker struct {
	localVersion  string
	remoteVersion string
	skipCheck     bool
}

func newVersionChecker(opts *InjectOptions) *versionChecker {
	return &versionChecker{
		localVersion:  opts.LocalVersion,
		remoteVersion: opts.RemoteVersion,
		skipCheck:     opts.SkipVersionCheck,
	}
}

func (vc *versionChecker) buildExistsCheck(agentPath string) string {
	if vc.skipCheck {
		return fmt.Sprintf(`! [ -x "%s" ]`, agentPath)
	}
	return fmt.Sprintf(`! { [ -x "%s" ] && [ "$("%s" version 2>/dev/null)" = "%s" ]; }`,
		agentPath, agentPath, vc.remoteVersion)
}

func (vc *versionChecker) validateRemoteAgent(
	ctx context.Context,
	exec inject.ExecFunc,
	agentPath string,
	log log.Logger,
) (string, error) {
	if vc.skipCheck {
		log.Debug("skipping version validation")
		return "", nil
	}

	buf := &bytes.Buffer{}
	versionCmd := fmt.Sprintf("%s version", agentPath)
	err := exec(ctx, versionCmd, nil, buf, io.Discard)
	if err != nil {
		return "", fmt.Errorf("failed to get remote agent version: %w", err)
	}

	actualVersion := strings.TrimSpace(buf.String())
	if actualVersion != vc.remoteVersion {
		return actualVersion, fmt.Errorf("version mismatch: expected %s, got %s",
			vc.remoteVersion, actualVersion)
	}

	log.WithFields(logrus.Fields{
		"expected": vc.remoteVersion,
		"actual":   actualVersion,
	}).Debug("remote agent version validated")

	return actualVersion, nil
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
	return filepath.Join(c.BaseDir, "devpod-linux-"+arch)
}

func (c *BinaryCache) atomicWrite(path string, data io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
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

	if err := file.Close(); err != nil {
		_ = os.Remove(temp)
		return err
	}
	return os.Rename(temp, path)
}

type InjectSource struct{}

func (s *InjectSource) GetBinary(ctx context.Context, arch string) (io.ReadCloser, error) {
	if !s.matchesCurrentRuntime(arch) {
		return nil, ErrArchMismatch
	}
	return s.openCurrentExecutable()
}

func (s *InjectSource) matchesCurrentRuntime(arch string) bool {
	return runtime.GOOS == "linux" && runtime.GOARCH == arch
}

func (s *InjectSource) openCurrentExecutable() (io.ReadCloser, error) {
	path, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return os.Open(path)
}

func (s *InjectSource) SourceName() string {
	return "local_executable"
}

type FileCacheSource struct {
	Cache *BinaryCache
}

func (s *FileCacheSource) GetBinary(ctx context.Context, arch string) (io.ReadCloser, error) {
	return s.Cache.Get(arch)
}

func (s *FileCacheSource) SourceName() string {
	return "local_cache"
}

type HTTPDownloadSource struct {
	BaseURL string
	Cache   *BinaryCache
}

func (s *HTTPDownloadSource) GetBinary(ctx context.Context, arch string) (io.ReadCloser, error) {
	url := fmt.Sprintf("%s/devpod-linux-%s", s.BaseURL, arch)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := devpodhttp.GetHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("bad status: %s", resp.Status)
	}

	if s.Cache != nil {
		return s.cacheAndReturn(arch, resp.Body)
	}

	return resp.Body, nil
}

func (s *HTTPDownloadSource) cacheAndReturn(arch string, body io.ReadCloser) (io.ReadCloser, error) {
	pr, pw := io.Pipe()

	go func() {
		defer func() {
			_ = body.Close()
		}()

		streamOnly := func() {
			if _, err := io.Copy(pw, body); err != nil {
				_ = pw.CloseWithError(err)
			} else {
				_ = pw.Close()
			}
		}

		cachePath := s.Cache.pathFor(arch)
		if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
			streamOnly()
			return
		}

		file, err := os.CreateTemp(filepath.Dir(cachePath), "devpod-agent-*.tmp")
		if err != nil {
			streamOnly()
			return
		}
		tmpPath := file.Name()

		success := false
		defer func() {
			if !success {
				_ = os.Remove(tmpPath)
			}
		}()

		mw := io.MultiWriter(file, pw)
		if _, err := io.Copy(mw, body); err != nil {
			_ = file.Close()
			_ = pw.CloseWithError(err)
			return
		}

		_ = pw.Close()
		_ = file.Sync()
		_ = file.Close()

		if err := os.Rename(tmpPath, cachePath); err == nil {
			success = true
		}
	}()

	return pr, nil
}

func (s *HTTPDownloadSource) SourceName() string {
	return "http_download"
}
