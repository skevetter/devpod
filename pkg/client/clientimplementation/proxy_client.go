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
	platformclient "github.com/skevetter/devpod/pkg/platform/client"
	"github.com/skevetter/devpod/pkg/provider"
	"github.com/skevetter/log"
)

var (
	DevPodDebug = "DEVPOD_DEBUG"

	DevPodPlatformOptions = "DEVPOD_PLATFORM_OPTIONS"

	DevPodFlagsUp     = "DEVPOD_FLAGS_UP"
	DevPodFlagsSsh    = "DEVPOD_FLAGS_SSH"
	DevPodFlagsDelete = "DEVPOD_FLAGS_DELETE"
	DevPodFlagsStatus = "DEVPOD_FLAGS_STATUS"
)

func NewProxyClient(devPodConfig *config.Config, prov *provider.ProviderConfig, workspace *provider.Workspace, log log.Logger) (client.ProxyClient, error) {
	return &proxyClient{
		devPodConfig: devPodConfig,
		config:       prov,
		workspace:    workspace,
		log:          log,
	}, nil
}

type proxyClient struct {
	m sync.Mutex

	workspaceLockOnce sync.Once
	workspaceLock     *flock.Flock

	devPodConfig *config.Config
	config       *provider.ProviderConfig
	workspace    *provider.Workspace
	log          log.Logger
}

func (s *proxyClient) Lock(ctx context.Context) error {
	s.initLock()

	// try to lock workspace
	s.log.Debugf("Acquire workspace lock...")
	err := tryLock(ctx, s.workspaceLock, "workspace", s.log)
	if err != nil {
		return fmt.Errorf("error locking workspace %w", err)
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
	for time.Since(now) < time.Minute*5 {
		locked, err := lock.TryLock()
		if err != nil {
			return err
		} else if locked {
			return nil
		}

		select {
		case <-time.After(time.Second):
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
			panic(fmt.Errorf("get workspaces dir %w", err))
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
		return fmt.Errorf("parse options %w", err)
	}

	workspace, err := options.ResolveAndSaveOptionsProxy(ctx, s.devPodConfig, s.config, s.workspace, userOptions, s.log)
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
	err := RunCommandWithBinaries(CommandOptions{
		Ctx:       ctx,
		Name:      "createWorkspace",
		Command:   s.config.Exec.Proxy.Create.Workspace,
		Context:   s.workspace.Context,
		Workspace: s.workspace,
		Machine:   nil,
		Options:   s.devPodConfig.ProviderOptions(s.config.Name),
		Config:    s.config,
		ExtraEnv:  nil,
		Stdin:     stdin,
		Stdout:    stdout,
		Stderr:    stderr,
		Log:       s.log,
	})
	if err != nil {
		return fmt.Errorf("create remote workspace  %w", err)
	}

	return nil
}

func (s *proxyClient) Up(ctx context.Context, opt client.UpOptions) error {
	writer, _ := devpodlog.PipeJSONStream(s.log.ErrorStreamOnly())
	defer func() { _ = writer.Close() }()

	opts := EncodeOptions(opt.CLIOptions, DevPodFlagsUp)
	if opt.Debug {
		opts["DEBUG"] = "true"
	}

	// check if the provider is outdated
	providerOptions := s.devPodConfig.ProviderOptions(s.config.Name)
	if providerOptions["LOFT_CONFIG"].Value != "" {
		baseClient, err := platformclient.InitClientFromPath(ctx, providerOptions["LOFT_CONFIG"].Value)
		if err != nil {
			return fmt.Errorf("error initializing platform client %w", err)
		}

		version, err := baseClient.Version()
		if err != nil {
			return fmt.Errorf("error retrieving platform version %w", err)
		}

		// check if the version is lower than v4.3.0-devpod.alpha.19
		parsedVersion, err := semver.Parse(strings.TrimPrefix(version.DevPodVersion, "v"))
		if err != nil {
			return fmt.Errorf("error parsing platform version %w", err)
		}

		// if devpod version is greater than 0.7.0 we error here
		if parsedVersion.GE(semver.MustParse("0.6.99")) {
			return fmt.Errorf("you are using an outdated provider version for this platform. Please disconnect and reconnect the platform to update the provider")
		}
	}

	err := RunCommandWithBinaries(CommandOptions{
		Ctx:       ctx,
		Name:      "up",
		Command:   s.config.Exec.Proxy.Up,
		Context:   s.workspace.Context,
		Workspace: s.workspace,
		Machine:   nil,
		Options:   providerOptions,
		Config:    s.config,
		ExtraEnv:  opts,
		Stdin:     opt.Stdin,
		Stdout:    opt.Stdout,
		Stderr:    writer,
		Log:       s.log.ErrorStreamOnly(),
	})
	if err != nil {
		return fmt.Errorf("error running devpod up %w", err)
	}

	return nil
}

func (s *proxyClient) Ssh(ctx context.Context, opt client.SshOptions) error {
	writer, _ := devpodlog.PipeJSONStream(s.log.ErrorStreamOnly())
	defer func() { _ = writer.Close() }()

	err := RunCommandWithBinaries(CommandOptions{
		Ctx:       ctx,
		Name:      "ssh",
		Command:   s.config.Exec.Proxy.Ssh,
		Context:   s.workspace.Context,
		Workspace: s.workspace,
		Machine:   nil,
		Options:   s.devPodConfig.ProviderOptions(s.config.Name),
		Config:    s.config,
		ExtraEnv:  EncodeOptions(opt, DevPodFlagsSsh),
		Stdin:     opt.Stdin,
		Stdout:    opt.Stdout,
		Stderr:    writer,
		Log:       s.log.ErrorStreamOnly(),
	})
	if err != nil {
		return err
	}

	return nil
}

func (s *proxyClient) Delete(ctx context.Context, opt client.DeleteOptions) error {
	s.m.Lock()
	defer s.m.Unlock()

	writer, _ := devpodlog.PipeJSONStream(s.log.ErrorStreamOnly())
	defer func() { _ = writer.Close() }()

	var gracePeriod *time.Duration
	if opt.GracePeriod != "" {
		duration, err := time.ParseDuration(opt.GracePeriod)
		if err == nil {
			gracePeriod = &duration
		}
	}

	// kill the command after the grace period
	if gracePeriod != nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, *gracePeriod)
		defer cancel()
	}

	err := RunCommandWithBinaries(CommandOptions{
		Ctx:       ctx,
		Name:      "delete",
		Command:   s.config.Exec.Proxy.Delete,
		Context:   s.workspace.Context,
		Workspace: s.workspace,
		Machine:   nil,
		Options:   s.devPodConfig.ProviderOptions(s.config.Name),
		Config:    s.config,
		ExtraEnv:  EncodeOptions(opt, DevPodFlagsDelete),
		Stdin:     nil,
		Stdout:    writer,
		Stderr:    writer,
		Log:       s.log,
	})
	if err != nil {
		if !opt.Force {
			return fmt.Errorf("error deleting workspace %w", err)
		}

		s.log.Errorf("Error deleting workspace: %v", err)
	}

	return DeleteWorkspaceFolder(DeleteWorkspaceFolderParams{
		Context:              s.workspace.Context,
		WorkspaceID:          s.workspace.ID,
		SSHConfigPath:        s.workspace.SSHConfigPath,
		SSHConfigIncludePath: s.workspace.SSHConfigIncludePath,
	}, s.log)
}

func (s *proxyClient) Stop(ctx context.Context, opt client.StopOptions) error {
	s.m.Lock()
	defer s.m.Unlock()

	writer, _ := devpodlog.PipeJSONStream(s.log.ErrorStreamOnly())
	defer func() { _ = writer.Close() }()

	err := RunCommandWithBinaries(CommandOptions{
		Ctx:       ctx,
		Name:      "stop",
		Command:   s.config.Exec.Proxy.Stop,
		Context:   s.workspace.Context,
		Workspace: s.workspace,
		Machine:   nil,
		Options:   s.devPodConfig.ProviderOptions(s.config.Name),
		Config:    s.config,
		ExtraEnv:  nil,
		Stdin:     nil,
		Stdout:    writer,
		Stderr:    writer,
		Log:       s.log,
	})
	if err != nil {
		return fmt.Errorf("error stopping container %w", err)
	}

	return nil
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
	err := RunCommandWithBinaries(CommandOptions{
		Ctx:       ctx,
		Name:      "updateWorkspace",
		Command:   s.config.Exec.Proxy.Update.Workspace,
		Context:   s.workspace.Context,
		Workspace: s.workspace,
		Machine:   nil,
		Options:   s.devPodConfig.ProviderOptions(s.config.Name),
		Config:    s.config,
		ExtraEnv:  nil,
		Stdin:     os.Stdin,
		Stdout:    os.Stdout,
		Stderr:    os.Stderr,
		Log:       s.log.ErrorStreamOnly(),
	})
	if err != nil {
		return err
	}

	return nil
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
