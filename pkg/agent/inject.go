package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/pkg/inject"
	"github.com/skevetter/devpod/pkg/shell"
	"github.com/skevetter/devpod/pkg/version"
	"github.com/skevetter/log"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
)

var (
	ErrBinaryNotFound = errors.New("agent binary not found")
	ErrInjectTimeout  = errors.New("injection timeout")
	ErrArchMismatch   = errors.New("architecture mismatch")
)

const (
	osLinux = "linux"
)

var waitForInstanceConnectionTimeout = time.Minute * 5

// InjectOptions defines the parameters for injecting the DevPod agent into a remote environment.
type InjectOptions struct {
	// Ctx is the context for the injection operation. Required.
	Ctx context.Context
	// Exec is the function used to execute commands on the remote machine. Required.
	Exec inject.ExecFunc
	// Log is the logger for capturing injection output.
	// Required.
	Log log.Logger

	// IsLocal indicates if the injection target is the local machine.
	IsLocal bool
	// RemoteAgentPath is the path where the agent binary should be placed on the remote machine.
	// Defaults to RemoteDevPodHelperLocation.
	RemoteAgentPath string
	// DownloadURL is the base URL to download the agent binary from. Defaults to DefaultAgentDownloadURL().
	DownloadURL string
	// PreferDownloadFromRemoteUrl forces downloading the agent even if a local binary is available.
	// Defaults to true for release versions, false for dev versions.
	PreferDownloadFromRemoteUrl *bool
	// Timeout is the maximum duration to wait for the injection to complete. Defaults to 5 minutes.
	Timeout time.Duration

	// Command is the command to execute after successful injection.
	Command string

	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	// LocalVersion is the version of the local DevPod binary.
	// Defaults to version.GetVersion().
	LocalVersion string
	// RemoteVersion is the expected version of the remote agent.
	// Defaults to LocalVersion.
	RemoteVersion string
	// SkipVersionCheck disables the validation of the remote agent's version.
	// Defaults to false, unless DEVPOD_AGENT_URL is set.
	SkipVersionCheck bool
}

func (o *InjectOptions) ApplyDefaults() {
	o.applyPathDefaults()
	o.applyURLDefaults()
	o.applyPreferDownloadDefaults()
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

func (o *InjectOptions) applyPathDefaults() {
	if o.RemoteAgentPath == "" {
		o.RemoteAgentPath = RemoteDevPodHelperLocation
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
}

func (o *InjectOptions) applyURLDefaults() {
	if o.DownloadURL == "" {
		o.DownloadURL = DefaultAgentDownloadURL()
	}

	if strings.Contains(o.DownloadURL, "github.com") && strings.Contains(o.DownloadURL, "/releases/tag/") {
		normalizedDownloadUrl := strings.Replace(o.DownloadURL, "/releases/tag/", "/releases/download/", 1)
		o.Log.Warnf("download URL %s is a tag URL, normalizing to download URL %s", o.DownloadURL, normalizedDownloadUrl)
		o.DownloadURL = normalizedDownloadUrl
	}
}

func (o *InjectOptions) applyPreferDownloadDefaults() {
	if o.PreferDownloadFromRemoteUrl != nil {
		return
	}

	isDefaultURL := o.DownloadURL == DefaultAgentDownloadURL()
	hasCustomAgentURL := os.Getenv(EnvDevPodAgentURL) != "" || !isDefaultURL

	preferDownloadEnv := os.Getenv(EnvDevPodAgentPreferDownload)
	switch {
	case preferDownloadEnv != "":
		o.applyEnvPreference(preferDownloadEnv)
	case hasCustomAgentURL:
		o.PreferDownloadFromRemoteUrl = Bool(true)
		o.SkipVersionCheck = true
	case version.GetVersion() == version.DevVersion:
		o.PreferDownloadFromRemoteUrl = Bool(false)
		o.SkipVersionCheck = true
	default:
		o.PreferDownloadFromRemoteUrl = Bool(true)
	}
}

func (o *InjectOptions) applyEnvPreference(preferDownloadEnv string) {
	pref, err := strconv.ParseBool(preferDownloadEnv)
	if err != nil {
		o.Log.Warnf("failed to parse %s, using default", EnvDevPodAgentPreferDownload)
		pref = true
	}
	o.PreferDownloadFromRemoteUrl = Bool(pref)
	o.SkipVersionCheck = true
}

func InjectAgent(opts *InjectOptions) error {
	opts.ApplyDefaults()
	if err := opts.Validate(); err != nil {
		return err
	}

	if opts.IsLocal {
		return injectLocally(opts)
	}

	vc := newVersionChecker(opts)
	bm := NewBinaryManager(opts.Log, opts.DownloadURL)

	backoff := wait.Backoff{
		Steps:    30,
		Duration: 10 * time.Second,
		Factor:   1.5,
		Jitter:   0.1,
		Cap:      60 * time.Second,
	}

	opts.Log.Debug("starting agent injection")
	return retry.OnError(backoff, func(err error) bool {
		if opts.Ctx.Err() != nil {
			return false
		}
		opts.Log.Debugf("retrying injection: %v", err)
		return true
	}, func() error {
		return injectAgent(&injectContext{
			opts: opts,
			bm:   bm,
			vc:   vc,
		})
	})
}

func injectLocally(opts *InjectOptions) error {
	if opts.Command == "" {
		return nil
	}
	opts.Log.Debug("execute command locally")
	return shell.RunEmulatedShell(opts.Ctx, opts.Command, opts.Stdin, opts.Stdout, opts.Stderr, nil)
}

type injectContext struct {
	opts *InjectOptions
	bm   *BinaryManager
	vc   *versionChecker
}

func injectAgent(ctx *injectContext) error {
	opts := ctx.opts

	buf := &bytes.Buffer{}
	stderr := setupStderr(opts, buf)
	binaryLoader := createBinaryLoader(ctx)
	scriptParams := buildScriptParams(ctx)

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
		return handleInjectError(err, wasExecuted, buf)
	}

	return performVersionCheck(ctx)
}

func setupStderr(opts *InjectOptions, buf *bytes.Buffer) io.Writer {
	if opts.Stderr != nil {
		return io.MultiWriter(opts.Stderr, buf)
	}
	return buf
}

func createBinaryLoader(ctx *injectContext) func(bool) (io.ReadCloser, error) {
	return func(arm bool) (io.ReadCloser, error) {
		arch := "amd64"
		if arm {
			arch = "arm64"
		}
		return ctx.bm.AcquireBinary(ctx.opts.Ctx, arch)
	}
}

func buildScriptParams(ctx *injectContext) *inject.Params {
	opts := ctx.opts
	return &inject.Params{
		Command:             opts.Command,
		AgentRemotePath:     opts.RemoteAgentPath,
		DownloadURLs:        inject.NewDownloadURLs(opts.DownloadURL),
		ExistsCheck:         ctx.vc.buildExistsCheck(opts.RemoteAgentPath),
		PreferAgentDownload: *opts.PreferDownloadFromRemoteUrl,
		ShouldChmodPath:     true,
	}
}

func handleInjectError(err error, wasExecuted bool, buf *bytes.Buffer) error {
	if wasExecuted {
		return &InjectError{
			Stage: InjectStageCommandExecution,
			Cause: fmt.Errorf("%w: %s", err, buf.String()),
		}
	}
	return &InjectError{Stage: InjectStageInject, Cause: err}
}

func performVersionCheck(ctx *injectContext) error {
	opts := ctx.opts

	detectedVersion, err := ctx.vc.detectRemoteAgentVersion(opts.Ctx, opts.Exec, opts.RemoteAgentPath, opts.Log)

	if !opts.SkipVersionCheck {
		if err != nil {
			return &InjectError{Stage: InjectStageVersionCheck, Cause: err}
		}
	}

	if detectedVersion != "" && !opts.SkipVersionCheck {
		opts.Log.Debugf("detected remote agent version: %s", detectedVersion)
	}

	return nil
}

type InjectStage string

const (
	InjectStageInject           InjectStage = "inject"
	InjectStageCommandExecution InjectStage = "command execution"
	InjectStageVersionCheck     InjectStage = "version check"
)

type InjectError struct {
	Stage InjectStage
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

func (vc *versionChecker) detectRemoteAgentVersion(
	ctx context.Context,
	exec inject.ExecFunc,
	agentPath string,
	log log.Logger,
) (string, error) {
	buf := &bytes.Buffer{}
	versionCmd := fmt.Sprintf("%s version", agentPath)
	err := exec(ctx, versionCmd, nil, buf, io.Discard)
	if err != nil {
		return "", fmt.Errorf("failed to get remote agent version: %w", err)
	}

	actualVersion := strings.TrimSpace(buf.String())

	if vc.skipCheck {
		log.Debugf("skipping version validation, detected version: %s", actualVersion)
		return actualVersion, nil
	}

	if actualVersion != vc.remoteVersion {
		log.WithFields(logrus.Fields{
			"expectedVersion": vc.remoteVersion,
			"actualVersion":   actualVersion,
			"agentPath":       agentPath,
		}).Warn("the remote agent version does not match the expected version. " +
			"If your workspace fails to deploy, you may need to manually remove " +
			"the existing agent and redeploy.")
	} else {
		log.Debug("remote agent version matches expected version")
	}

	return actualVersion, nil
}
