package devcontainer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"al.essio.dev/pkg/shellescape"
	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/pkg/agent"
	"github.com/skevetter/devpod/pkg/agent/tunnelserver"
	"github.com/skevetter/devpod/pkg/compress"
	"github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/skevetter/devpod/pkg/devcontainer/crane"
	"github.com/skevetter/devpod/pkg/devcontainer/sshtunnel"
	"github.com/skevetter/devpod/pkg/driver"
	"github.com/skevetter/devpod/pkg/ide"
	provider2 "github.com/skevetter/devpod/pkg/provider"
	"github.com/skevetter/log"
)

const (
	stringTrue        = "true"
	stringFalse       = "false"
	containerRootUser = "root"
)

type setupContainerParams struct {
	rawConfig           *config.DevContainerConfig
	containerDetails    *config.ContainerDetails
	mergedConfig        *config.MergedDevContainerConfig
	substitutionContext *config.SubstitutionContext
	timeout             time.Duration
}

type setupInfo struct {
	result                    *config.Result
	compressed                string
	workspaceConfigCompressed string
}

func (r *runner) setupContainer(ctx context.Context, params *setupContainerParams) (*config.Result, error) {
	if err := r.injectAgentIntoContainer(ctx, params.timeout); err != nil {
		return nil, err
	}
	r.Log.Debugf("injected into container")
	defer r.Log.Debugf("done setting up container")

	info, err := r.prepareSetupInfo(params)
	if err != nil {
		return nil, err
	}

	setupCommand := r.buildSetupCommand(info.compressed, info.workspaceConfigCompressed)

	return r.executeSetup(ctx, info.result, setupCommand)
}

func (r *runner) injectAgentIntoContainer(ctx context.Context, timeout time.Duration) error {
	err := agent.InjectAgent(&agent.InjectOptions{
		Ctx: ctx,
		Exec: func(ctx context.Context, command string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
			return r.Driver.CommandDevContainer(ctx, r.ID, containerRootUser, command, stdin, stdout, stderr)
		},
		IsLocal:                     false,
		RemoteAgentPath:             agent.ContainerDevPodHelperLocation,
		DownloadURL:                 agent.DefaultAgentDownloadURL(),
		PreferDownloadFromRemoteUrl: agent.Bool(false),
		Log:                         r.Log,
		Timeout:                     timeout,
	})
	if err != nil {
		return fmt.Errorf("inject agent: %w", err)
	}
	return nil
}

func (r *runner) prepareSetupInfo(params *setupContainerParams) (*setupInfo, error) {
	result := r.buildResult(params)

	compressed, err := r.compressResult(result)
	if err != nil {
		return nil, err
	}

	workspaceConfigCompressed, err := r.compressWorkspaceConfig()
	if err != nil {
		return nil, err
	}

	return &setupInfo{
		result:                    result,
		compressed:                compressed,
		workspaceConfigCompressed: workspaceConfigCompressed,
	}, nil
}

func (r *runner) buildResult(params *setupContainerParams) *config.Result {
	result := &config.Result{
		DevContainerConfigWithPath: &config.DevContainerConfigWithPath{
			Config: params.rawConfig,
			Path:   getRelativeDevContainerJson(params.rawConfig.Origin, r.LocalWorkspaceFolder),
		},
		MergedConfig:        params.mergedConfig,
		SubstitutionContext: params.substitutionContext,
		ContainerDetails:    params.containerDetails,
	}

	if r.WorkspaceConfig.Agent.Local == stringTrue && r.WorkspaceConfig.CLIOptions.Platform.Enabled {
		result.MergedConfig.Mounts = filterWorkspaceMounts(result.MergedConfig.Mounts, r.WorkspaceConfig.ContentFolder, r.Log)
	}

	return result
}

func (r *runner) compressResult(result *config.Result) (string, error) {
	marshalled, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("marshal result: %w", err)
	}
	compressed, err := compress.Compress(string(marshalled))
	if err != nil {
		return "", fmt.Errorf("compress result: %w", err)
	}
	return compressed, nil
}

func (r *runner) compressWorkspaceConfig() (string, error) {
	workspaceConfig := &provider2.ContainerWorkspaceInfo{
		IDE:              r.WorkspaceConfig.Workspace.IDE,
		CLIOptions:       r.WorkspaceConfig.CLIOptions,
		Dockerless:       r.WorkspaceConfig.Agent.Dockerless,
		ContainerTimeout: r.WorkspaceConfig.Agent.ContainerTimeout,
		Source:           r.WorkspaceConfig.Workspace.Source,
		Agent:            r.WorkspaceConfig.Agent,
		ContentFolder:    r.WorkspaceConfig.ContentFolder,
	}
	if crane.ShouldUse(&r.WorkspaceConfig.CLIOptions) && r.WorkspaceConfig.Workspace.Source.GitRepository != "" {
		workspaceConfig.PullFromInsideContainer = stringTrue
	}

	workspaceConfigRaw, err := json.Marshal(workspaceConfig)
	if err != nil {
		return "", fmt.Errorf("marshal workspace config: %w", err)
	}
	compressed, err := compress.Compress(string(workspaceConfigRaw))
	if err != nil {
		return "", fmt.Errorf("compress workspace config: %w", err)
	}
	return compressed, nil
}

func (r *runner) buildSetupCommand(compressed, workspaceConfigCompressed string) string {
	r.Log.Infof("setting up container")
	args := []string{
		shellescape.Quote(agent.ContainerDevPodHelperLocation),
		"agent", "container", "setup",
		"--setup-info", shellescape.Quote(compressed),
		"--container-workspace-info", shellescape.Quote(workspaceConfigCompressed),
	}

	r.addSetupFlags(&args)
	return strings.Join(args, " ")
}

func (r *runner) addSetupFlags(args *[]string) {
	_, isDockerDriver := r.Driver.(driver.DockerDriver)

	r.addChownFlag(args, isDockerDriver)
	r.addDriverFlags(args, isDockerDriver)
	r.addPlatformFlags(args)
	r.addDebugFlag(args)
}

func (r *runner) addChownFlag(args *[]string, isDockerDriver bool) {
	if runtime.GOOS == "linux" || !isDockerDriver {
		*args = append(*args, "--chown-workspace")
	}
}

func (r *runner) addDriverFlags(args *[]string, isDockerDriver bool) {
	if !isDockerDriver {
		*args = append(*args, "--stream-mounts")
	}
	if r.WorkspaceConfig.Agent.InjectGitCredentials != stringFalse {
		*args = append(*args, "--inject-git-credentials")
	}
}

func (r *runner) addPlatformFlags(args *[]string) {
	platform := r.WorkspaceConfig.CLIOptions.Platform
	if platform.AccessKey != "" {
		*args = append(*args, "--access-key", shellescape.Quote(platform.AccessKey))
	}
	if platform.WorkspaceHost != "" {
		*args = append(*args, "--workspace-host", shellescape.Quote(platform.WorkspaceHost))
	}
	if platform.PlatformHost != "" {
		*args = append(*args, "--platform-host", shellescape.Quote(platform.PlatformHost))
	}
}

func (r *runner) addDebugFlag(args *[]string) {
	if r.isDebugMode() {
		*args = append(*args, "--debug")
	}
}

func (r *runner) isDebugMode() bool {
	return r.Log.GetLevel() == logrus.DebugLevel
}

func (r *runner) executeSetup(ctx context.Context, result *config.Result, setupCommand string) (*config.Result, error) {
	runSetupServer := func(ctx context.Context, stdin io.WriteCloser, stdout io.Reader) (*config.Result, error) {
		return tunnelserver.RunSetupServer(
			ctx,
			stdout,
			stdin,
			r.WorkspaceConfig.Agent.InjectGitCredentials != stringFalse,
			r.WorkspaceConfig.Agent.InjectDockerCredentials != stringFalse,
			config.GetMounts(result),
			r.Log,
			tunnelserver.WithPlatformOptions(&r.WorkspaceConfig.CLIOptions.Platform),
		)
	}

	sshTunnelCmd := r.buildSSHTunnelCommand()

	agentInjectFunc := func(
		cancelCtx context.Context,
		sshCmd string,
		sshTunnelStdinReader, sshTunnelStdoutWriter *os.File,
		writer io.WriteCloser,
	) error {
		return r.Driver.CommandDevContainer(
			cancelCtx,
			r.ID,
			containerRootUser,
			sshCmd,
			sshTunnelStdinReader,
			sshTunnelStdoutWriter,
			writer,
		)
	}

	return sshtunnel.ExecuteCommand(
		ctx,
		nil,
		false,
		agentInjectFunc,
		sshTunnelCmd,
		setupCommand,
		r.Log,
		runSetupServer,
	)
}

func (r *runner) buildSSHTunnelCommand() string {
	args := []string{
		shellescape.Quote(agent.ContainerDevPodHelperLocation),
		"helper", "ssh-server", "--stdio",
	}

	if ide.ReusesAuthSock(r.WorkspaceConfig.Workspace.IDE.Name) {
		args = append(args, "--reuse-ssh-auth-sock", shellescape.Quote(r.WorkspaceConfig.CLIOptions.SSHAuthSockID))
	}
	if r.isDebugMode() {
		args = append(args, "--debug")
	}
	return strings.Join(args, " ")
}

func getRelativeDevContainerJson(origin, localWorkspaceFolder string) string {
	relativePath := strings.TrimPrefix(filepath.ToSlash(origin), filepath.ToSlash(localWorkspaceFolder))
	return strings.TrimPrefix(relativePath, "/")
}

func filterWorkspaceMounts(mounts []*config.Mount, baseFolder string, log log.Logger) []*config.Mount {
	retMounts := []*config.Mount{}
	for _, mount := range mounts {
		rel, err := filepath.Rel(baseFolder, mount.Source)
		if err != nil || strings.Contains(rel, "..") {
			log.Infof(
				"dropping workspace mount %s because it possibly accesses data outside of its content directory",
				mount.Source,
			)
			continue
		}

		retMounts = append(retMounts, mount)
	}

	return retMounts
}
