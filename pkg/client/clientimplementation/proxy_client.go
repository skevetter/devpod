package clientimplementation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/blang/semver/v4"
	"github.com/gofrs/flock"
	"github.com/loft-sh/api/v4/pkg/devpod"
	"github.com/skevetter/devpod/pkg/client"
	"github.com/skevetter/devpod/pkg/config"
	devpodlog "github.com/skevetter/devpod/pkg/log"
	"github.com/skevetter/devpod/pkg/options"
	"github.com/skevetter/devpod/pkg/options/resolver"
	platformclient "github.com/skevetter/devpod/pkg/platform/client"
	"github.com/skevetter/devpod/pkg/provider"
	"github.com/skevetter/devpod/pkg/types"
	"github.com/skevetter/log"
	"github.com/skevetter/log/terminal"
)

var (
	DevPodDebug = "DEVPOD_DEBUG"

	DevPodPlatformOptions = "DEVPOD_PLATFORM_OPTIONS"

	DevPodFlagsUp     = "DEVPOD_FLAGS_UP"
	DevPodFlagsSsh    = "DEVPOD_FLAGS_SSH"
	DevPodFlagsDelete = "DEVPOD_FLAGS_DELETE"
	DevPodFlagsStatus = "DEVPOD_FLAGS_STATUS"
)

const (
	lockTimeout = 5 * time.Minute
	lockRetry   = time.Second
)

func NewProxyClient(devPodConfig *config.Config, prov *provider.ProviderConfig, workspace *provider.Workspace, log log.Logger) (client.ProxyClient, error) {
	pc := &proxyClient{
		devPodConfig: devPodConfig,
		config:       prov,
		workspace:    workspace,
		log:          log,
	}
	pc.executor = &proxyExecutor{client: pc}
	return pc, nil
}

type proxyClient struct {
	m sync.Mutex

	workspaceLockOnce sync.Once
	workspaceLock     *flock.Flock

	devPodConfig *config.Config
	config       *provider.ProviderConfig
	workspace    *provider.Workspace
	log          log.Logger
	executor     *proxyExecutor
}

// proxyExecutor handles proxy command execution with common patterns.
type proxyExecutor struct {
	client *proxyClient
}

// execParams defines parameters for proxy command execution.
type execParams struct {
	name     string
	command  types.StrArray
	extraEnv map[string]string
	stdin    io.Reader
	stdout   io.Writer
	stderr   io.Writer
}

// execute runs a proxy command with common settings.
func (e *proxyExecutor) execute(ctx context.Context, params execParams) error {
	return RunCommandWithBinaries(CommandOptions{
		Ctx:       ctx,
		Name:      params.name,
		Command:   params.command,
		Context:   e.client.workspace.Context,
		Workspace: e.client.workspace,
		Options:   e.client.devPodConfig.ProviderOptions(e.client.config.Name),
		Config:    e.client.config,
		ExtraEnv:  params.extraEnv,
		Stdin:     params.stdin,
		Stdout:    params.stdout,
		Stderr:    params.stderr,
		Log:       e.client.log.ErrorStreamOnly(),
	})
}

// executeWithJSONLog runs a command with JSON log streaming.
func (e *proxyExecutor) executeWithJSONLog(ctx context.Context, params execParams) error {
	writer, _ := devpodlog.PipeJSONStream(e.client.log.ErrorStreamOnly())
	defer func() { _ = writer.Close() }()

	params.stderr = writer
	return e.execute(ctx, params)
}

func (s *proxyClient) Lock(ctx context.Context) error {
	s.initLock()

	// try to lock workspace
	s.log.Debugf("Acquire workspace lock...")
	err := tryLock(ctx, s.workspaceLock, "workspace", s.log)
	if err != nil {
		return fmt.Errorf("error locking workspace: %w", err)
	}
	s.log.Debugf("Acquired workspace lock...")

	return nil
}

func (s *proxyClient) Unlock() {
	s.initLock()

	// try to unlock workspace
	err := s.workspaceLock.Unlock()
	if err != nil {
		s.log.Warnf("Error unlocking workspace: %v", err)
	}
}

func tryLock(ctx context.Context, lock *flock.Flock, name string, log log.Logger) error {
	done := scheduleLogMessage(fmt.Sprintf("Trying to lock %s, seems like another process is running that blocks this %s", name, name), log)
	defer close(done)

	now := time.Now()
	for time.Since(now) < lockTimeout {
		locked, err := lock.TryLock()
		if err != nil {
			return err
		} else if locked {
			return nil
		}

		select {
		case <-time.After(lockRetry):
			continue
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return fmt.Errorf("timed out waiting to lock %s, seems like there is another process running on this machine that blocks it", name)
}

func (s *proxyClient) initLock() {
	s.workspaceLockOnce.Do(func() {
		s.m.Lock()
		defer s.m.Unlock()

		// get locks dir
		workspaceLocksDir, err := provider.GetLocksDir(s.workspace.Context)
		if err != nil {
			panic(fmt.Errorf("get workspaces dir: %w", err))
		}
		_ = os.MkdirAll(workspaceLocksDir, 0777)

		// create workspace lock
		s.workspaceLock = flock.New(filepath.Join(workspaceLocksDir, s.workspace.ID+".workspace.lock"))
	})
}

func (s *proxyClient) Provider() string {
	return s.config.Name
}

func (s *proxyClient) Workspace() string {
	s.m.Lock()
	defer s.m.Unlock()

	return s.workspace.ID
}

func (s *proxyClient) WorkspaceConfig() *provider.Workspace {
	s.m.Lock()
	defer s.m.Unlock()

	return provider.CloneWorkspace(s.workspace)
}

func (s *proxyClient) Context() string {
	return s.workspace.Context
}

func (s *proxyClient) RefreshOptions(ctx context.Context, userOptionsRaw []string, reconfigure bool) error {
	s.m.Lock()
	defer s.m.Unlock()

	userOptions, err := provider.ParseOptions(userOptionsRaw)
	if err != nil {
		return fmt.Errorf("parse options: %w", err)
	}

	workspace, err := options.ResolveAndSaveOptionsWorkspace(
		ctx,
		s.devPodConfig,
		s.config,
		s.workspace,
		userOptions,
		s.log,
		resolver.WithResolveSubOptions(),
	)
	if err != nil {
		return err
	}

	if reconfigure {
		err := s.updateInstance(ctx)
		if err != nil {
			return err
		}
	}

	s.workspace = workspace
	return nil
}

func (s *proxyClient) Create(ctx context.Context, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	err := s.executor.execute(ctx, execParams{
		name:    "createWorkspace",
		command: s.config.Exec.Proxy.Create.Workspace,
		stdin:   stdin,
		stdout:  stdout,
		stderr:  stderr,
	})
	if err != nil {
		return fmt.Errorf("create remote workspace : %w", err)
	}
	return nil
}

func (s *proxyClient) Ssh(ctx context.Context, opt client.SshOptions) error {
	return s.executor.executeWithJSONLog(ctx, execParams{
		name:     "ssh",
		command:  s.config.Exec.Proxy.Ssh,
		extraEnv: EncodeOptions(opt, DevPodFlagsSsh),
		stdin:    opt.Stdin,
		stdout:   opt.Stdout,
	})
}

func (s *proxyClient) Stop(ctx context.Context, opt client.StopOptions) error {
	s.m.Lock()
	defer s.m.Unlock()

	return s.executor.executeWithJSONLog(ctx, execParams{
		name:    "stop",
		command: s.config.Exec.Proxy.Stop,
		stdout:  io.Discard,
	})
}

func (s *proxyClient) Up(ctx context.Context, opt client.UpOptions) error {
	opts := EncodeOptions(opt.CLIOptions, DevPodFlagsUp)
	if opt.Debug {
		opts["DEBUG"] = "true"
	}

	providerOptions := s.devPodConfig.ProviderOptions(s.config.Name)
	if err := s.checkPlatformVersion(ctx, providerOptions); err != nil {
		return err
	}

	return s.executor.executeWithJSONLog(ctx, execParams{
		name:     "up",
		command:  s.config.Exec.Proxy.Up,
		extraEnv: opts,
		stdin:    opt.Stdin,
		stdout:   opt.Stdout,
	})
}

// checkPlatformVersion validates the platform provider version compatibility.
func (s *proxyClient) checkPlatformVersion(ctx context.Context, providerOptions map[string]config.OptionValue) error {
	loftConfig := providerOptions["LOFT_CONFIG"].Value
	if loftConfig == "" {
		return nil
	}

	baseClient, err := platformclient.InitClientFromPath(ctx, loftConfig)
	if err != nil {
		return fmt.Errorf("error initializing platform client: %w", err)
	}

	version, err := baseClient.Version()
	if err != nil {
		return fmt.Errorf("error retrieving platform version: %w", err)
	}

	parsedVersion, err := semver.Parse(strings.TrimPrefix(version.DevPodVersion, "v"))
	if err != nil {
		return fmt.Errorf("error parsing platform version: %w", err)
	}

	if parsedVersion.GE(semver.MustParse("0.6.99")) {
		return fmt.Errorf("you are using an outdated provider version for this platform. Please disconnect and reconnect the platform to update the provider")
	}

	return nil
}

func (s *proxyClient) Delete(ctx context.Context, opt client.DeleteOptions) error {
	s.m.Lock()
	defer s.m.Unlock()

	if opt.GracePeriod != "" {
		if duration, err := time.ParseDuration(opt.GracePeriod); err == nil {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, duration)
			defer cancel()
		}
	}

	err := s.executor.executeWithJSONLog(ctx, execParams{
		name:     "delete",
		command:  s.config.Exec.Proxy.Delete,
		extraEnv: EncodeOptions(opt, DevPodFlagsDelete),
		stdout:   io.Discard,
	})
	if err != nil && !opt.Force {
		return fmt.Errorf("error deleting workspace: %w", err)
	}
	if err != nil {
		s.log.Errorf("Error deleting workspace: %v", err)
	}

	return DeleteWorkspaceFolder(DeleteWorkspaceFolderParams{
		Context:              s.workspace.Context,
		WorkspaceID:          s.workspace.ID,
		SSHConfigPath:        s.workspace.SSHConfigPath,
		SSHConfigIncludePath: s.workspace.SSHConfigIncludePath,
	}, s.log)
}

func (s *proxyClient) Status(ctx context.Context, options client.StatusOptions) (client.Status, error) {
	s.m.Lock()
	defer s.m.Unlock()

	stdout := &bytes.Buffer{}
	buf := &bytes.Buffer{}
	err := RunCommandWithBinaries(CommandOptions{
		Ctx:       ctx,
		Name:      "status",
		Command:   s.config.Exec.Proxy.Status,
		Context:   s.workspace.Context,
		Workspace: s.workspace,
		Machine:   nil,
		Options:   s.devPodConfig.ProviderOptions(s.config.Name),
		Config:    s.config,
		ExtraEnv:  EncodeOptions(options, DevPodFlagsStatus),
		Stdin:     nil,
		Stdout:    io.MultiWriter(stdout, buf),
		Stderr:    buf,
		Log:       s.log.ErrorStreamOnly(),
	})
	if err != nil {
		return client.StatusNotFound, fmt.Errorf("error retrieving container status: %s%w", buf.String(), err)
	}

	devpodlog.ReadJSONStream(bytes.NewReader(buf.Bytes()), s.log.ErrorStreamOnly())
	status := &client.WorkspaceStatus{}
	err = json.Unmarshal(stdout.Bytes(), status)
	if err != nil {
		return client.StatusNotFound, fmt.Errorf("error parsing proxy command response: %s%w", stdout.String(), err)
	}

	// parse status
	return client.ParseStatus(status.State)
}

func (s *proxyClient) updateInstance(ctx context.Context) error {
	if !terminal.IsTerminalIn {
		return fmt.Errorf("unable to update instance through CLI if stdin is not a terminal")
	}

	return s.executor.execute(ctx, execParams{
		name:    "updateWorkspace",
		command: s.config.Exec.Proxy.Update.Workspace,
		stdin:   os.Stdin,
		stdout:  os.Stdout,
		stderr:  os.Stderr,
	})
}

func EncodeOptions(options any, name string) map[string]string {
	raw, _ := json.Marshal(options)
	return map[string]string{
		name: string(raw),
	}
}

func DecodeOptionsFromEnv(name string, into any) (bool, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return false, nil
	}

	return true, json.Unmarshal([]byte(raw), into)
}

func DecodePlatformOptionsFromEnv(into *devpod.PlatformOptions) error {
	raw := os.Getenv(DevPodPlatformOptions)
	if raw == "" {
		return nil
	}

	return json.Unmarshal([]byte(raw), into)
}
