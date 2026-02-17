package docker

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
	config2 "github.com/skevetter/devpod/pkg/config"
	"github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/skevetter/devpod/pkg/docker"
	"github.com/skevetter/devpod/pkg/driver"
	"github.com/skevetter/devpod/pkg/ide/jetbrains"
)

type runArgsBuilder struct {
	args   []string
	driver *dockerDriver
	params *driver.RunDockerDevContainerParams
}

func (d *dockerDriver) buildRunArgs(params *driver.RunDockerDevContainerParams, helper *docker.DockerHelper) ([]string, error) {
	b := &runArgsBuilder{
		args:   []string{"run"},
		driver: d,
		params: params,
	}

	if !helper.IsNerdctl() {
		b.args = append(b.args, "--sig-proxy=false")
	}

	b.addPorts().
		addWorkspaceMount(helper).
		addUser().
		addEnv().
		addInit().
		addPrivileged()

	if err := b.addPodmanArgs(); err != nil {
		return nil, err
	}

	b.addCapabilities()

	if err := b.addMounts(); err != nil {
		return nil, err
	}

	b.addIDEMount().
		addLabels().
		addGPU().
		addRunArgs().
		addDetached().
		addEntrypoint().
		addImage()

	return b.args, nil
}

func (b *runArgsBuilder) addPorts() *runArgsBuilder {
	b.args = b.driver.addPortArgs(b.args, b.params.ParsedConfig)
	return b
}

func (b *runArgsBuilder) addWorkspaceMount(helper *docker.DockerHelper) *runArgsBuilder {
	b.args = b.driver.addWorkspaceMountArgs(b.args, b.params.Options, helper)
	return b
}

func (b *runArgsBuilder) addUser() *runArgsBuilder {
	b.args = b.driver.addUserArgs(b.args, b.params.Options)
	return b
}

func (b *runArgsBuilder) addEnv() *runArgsBuilder {
	b.args = b.driver.addEnvArgs(b.args, b.params.Options)
	return b
}

func (b *runArgsBuilder) addInit() *runArgsBuilder {
	b.args = b.driver.addInitArgs(b.args, b.params.Options)
	return b
}

func (b *runArgsBuilder) addPrivileged() *runArgsBuilder {
	b.args = b.driver.addPrivilegedArgs(b.args, b.params.Options)
	return b
}

func (b *runArgsBuilder) addPodmanArgs() error {
	podmanArgs, err := b.driver.getPodmanArgs(b.params.Options, b.params.ParsedConfig)
	if err != nil {
		return err
	}
	b.args = append(b.args, podmanArgs...)
	return nil
}

func (b *runArgsBuilder) addCapabilities() *runArgsBuilder {
	b.args = b.driver.addCapabilityArgs(b.args, b.params.Options)
	return b
}

func (b *runArgsBuilder) addMounts() error {
	args, err := b.driver.addMountArgs(b.args, b.params.Options)
	if err != nil {
		return err
	}
	b.args = args
	return nil
}

func (b *runArgsBuilder) addIDEMount() *runArgsBuilder {
	b.args = b.driver.addIDEMountArgs(b.args, b.params.IDE, b.params.IDEOptions)
	return b
}

func (b *runArgsBuilder) addLabels() *runArgsBuilder {
	b.args = b.driver.addLabelArgs(b.args, b.params.WorkspaceID, b.params.Options)
	return b
}

func (b *runArgsBuilder) addGPU() *runArgsBuilder {
	b.args = appendGPUOptions(b.params.ParsedConfig, b.driver, b.args)
	return b
}

func (b *runArgsBuilder) addRunArgs() *runArgsBuilder {
	b.args = append(b.args, b.params.ParsedConfig.RunArgs...)
	return b
}

func (b *runArgsBuilder) addDetached() *runArgsBuilder {
	b.args = append(b.args, "-d")
	return b
}

func (b *runArgsBuilder) addEntrypoint() *runArgsBuilder {
	b.args = b.driver.addEntrypointArgs(b.args, b.params.Options)
	return b
}

func (b *runArgsBuilder) addImage() *runArgsBuilder {
	b.args = append(b.args, b.params.Options.Image)
	b.args = append(b.args, b.params.Options.Cmd...)
	return b
}

func (d *dockerDriver) addPortArgs(args []string, parsedConfig *config.DevContainerConfig) []string {
	for _, appPort := range parsedConfig.AppPort {
		intPort, err := strconv.Atoi(appPort)
		if err != nil {
			args = append(args, "-p", appPort)
		} else {
			args = append(args, "-p", fmt.Sprintf("127.0.0.1:%d:%d", intPort, intPort))
		}
	}
	return args
}

func (d *dockerDriver) addWorkspaceMountArgs(args []string, options *driver.RunOptions, helper *docker.DockerHelper) []string {
	if options.WorkspaceMount != nil {
		workspacePath := d.EnsurePath(options.WorkspaceMount)
		mountPath := workspacePath.String()
		if helper.IsNerdctl() && strings.Contains(mountPath, ",consistency='consistent'") {
			mountPath = strings.Replace(mountPath, ",consistency='consistent'", "", 1)
		}
		args = append(args, "--mount", mountPath)
	}
	return args
}

func (d *dockerDriver) addUserArgs(args []string, options *driver.RunOptions) []string {
	if options.User != "" {
		args = append(args, "-u", options.User)
	}
	return args
}

func (d *dockerDriver) addEnvArgs(args []string, options *driver.RunOptions) []string {
	for k, v := range options.Env {
		args = append(args, "-e", k+"="+v)
	}
	return args
}

func (d *dockerDriver) addInitArgs(args []string, options *driver.RunOptions) []string {
	if options.Init != nil && *options.Init {
		args = append(args, "--init")
	}
	return args
}

func (d *dockerDriver) addPrivilegedArgs(args []string, options *driver.RunOptions) []string {
	if options.Privileged != nil && *options.Privileged {
		args = append(args, "--privileged")
	}
	return args
}

func (d *dockerDriver) addCapabilityArgs(args []string, options *driver.RunOptions) []string {
	for _, capAdd := range options.CapAdd {
		args = append(args, "--cap-add", capAdd)
	}
	for _, securityOpt := range options.SecurityOpt {
		args = append(args, "--security-opt", securityOpt)
	}
	return args
}

func (d *dockerDriver) addMountArgs(args []string, options *driver.RunOptions) ([]string, error) {
	for _, mount := range options.Mounts {
		if mount.Type == "bind" && mount.Source != "" {
			if _, err := os.Stat(mount.Source); os.IsNotExist(err) {
				return nil, fmt.Errorf("bind mount source path does not exist %s", mount.Source)
			}
		}
		args = append(args, "--mount", mount.String())
	}
	return args, nil
}

func (d *dockerDriver) addIDEMountArgs(args []string, ide string, ideOptions map[string]config2.OptionValue) []string {
	switch ide {
	case string(config2.IDEGoland):
		args = append(args, "--mount", jetbrains.NewGolandServer("", ideOptions, d.Log).GetVolume())
	case string(config2.IDERustRover):
		args = append(args, "--mount", jetbrains.NewRustRoverServer("", ideOptions, d.Log).GetVolume())
	case string(config2.IDEPyCharm):
		args = append(args, "--mount", jetbrains.NewPyCharmServer("", ideOptions, d.Log).GetVolume())
	case string(config2.IDEPhpStorm):
		args = append(args, "--mount", jetbrains.NewPhpStorm("", ideOptions, d.Log).GetVolume())
	case string(config2.IDEIntellij):
		args = append(args, "--mount", jetbrains.NewIntellij("", ideOptions, d.Log).GetVolume())
	case string(config2.IDECLion):
		args = append(args, "--mount", jetbrains.NewCLionServer("", ideOptions, d.Log).GetVolume())
	case string(config2.IDERider):
		args = append(args, "--mount", jetbrains.NewRiderServer("", ideOptions, d.Log).GetVolume())
	case string(config2.IDERubyMine):
		args = append(args, "--mount", jetbrains.NewRubyMineServer("", ideOptions, d.Log).GetVolume())
	case string(config2.IDEWebStorm):
		args = append(args, "--mount", jetbrains.NewWebStormServer("", ideOptions, d.Log).GetVolume())
	case string(config2.IDEDataSpell):
		args = append(args, "--mount", jetbrains.NewDataSpellServer("", ideOptions, d.Log).GetVolume())
	}
	return args
}

func (d *dockerDriver) addLabelArgs(args []string, workspaceId string, options *driver.RunOptions) []string {
	labels := append(config.GetDockerLabelForID(workspaceId), options.Labels...)
	for _, label := range labels {
		args = append(args, "-l", label)
	}
	return args
}

func (d *dockerDriver) addEntrypointArgs(args []string, options *driver.RunOptions) []string {
	if options.Entrypoint != "" {
		args = append(args, "--entrypoint", options.Entrypoint)
	}
	return args
}

func (d *dockerDriver) startContainer(ctx context.Context, args []string, writer io.Writer) error {
	d.Log.WithFields(logrus.Fields{"command": d.Docker.DockerCommand, "args": strings.Join(args, " ")}).Info("running docker command")
	err := d.Docker.Run(ctx, args, nil, writer, writer)
	if err != nil {
		d.Log.WithFields(logrus.Fields{"error": err, "command": d.Docker.DockerCommand, "args": strings.Join(args, " ")}).Error("docker container failed to start")
		return fmt.Errorf("failed to start dev container: %w", err)
	}
	return nil
}

func appendGPUOptions(parsedConfig *config.DevContainerConfig, d *dockerDriver, args []string) []string {
	if parsedConfig.HostRequirements != nil {
		gpuAvailable, _ := d.Docker.GPUSupportEnabled()
		enableGPU, warnIfMissing := parsedConfig.HostRequirements.ShouldEnableGPU(gpuAvailable)
		if enableGPU {
			args = append(args, "--gpus", "all")
		}
		if warnIfMissing {
			d.Log.Warn("GPU required but not available on host")
		}
	}
	return args
}

func (d *dockerDriver) getPodmanArgs(options *driver.RunOptions, parsedConfig *config.DevContainerConfig) ([]string, error) {
	if !d.Docker.IsPodman() {
		return []string{}, nil
	}

	var args []string
	args = d.addUsernsArgs(args, options)
	args = d.addIdMappingArgs(args, options)
	args = d.addKeepIdArgs(args, options, parsedConfig)
	return args, nil
}

func (d *dockerDriver) addUsernsArgs(args []string, options *driver.RunOptions) []string {
	if options.Userns != "" {
		args = append(args, "--userns", options.Userns)
	}
	return args
}

func (d *dockerDriver) addIdMappingArgs(args []string, options *driver.RunOptions) []string {
	for _, uidMap := range options.UidMap {
		args = append(args, "--uidmap", uidMap)
	}
	for _, gidMap := range options.GidMap {
		args = append(args, "--gidmap", gidMap)
	}
	return args
}

func (d *dockerDriver) addKeepIdArgs(args []string, options *driver.RunOptions, parsedConfig *config.DevContainerConfig) []string {
	if d.hasIdMapping(options, parsedConfig) || options.Userns != "" {
		return args
	}

	remoteUser := d.getRemoteUser(options, parsedConfig)
	if remoteUser != "root" && remoteUser != "0" && os.Getuid() != 0 {
		args = append(args, "--userns=keep-id")
	}
	return args
}

func (d *dockerDriver) hasIdMapping(options *driver.RunOptions, parsedConfig *config.DevContainerConfig) bool {
	if len(options.UidMap) > 0 || len(options.GidMap) > 0 {
		return true
	}

	if parsedConfig != nil {
		for _, arg := range parsedConfig.RunArgs {
			if strings.Contains(arg, "--uidmap") || strings.Contains(arg, "--gidmap") {
				return true
			}
		}
	}
	return false
}

func (d *dockerDriver) getRemoteUser(options *driver.RunOptions, parsedConfig *config.DevContainerConfig) string {
	if parsedConfig != nil {
		if parsedConfig.RemoteUser != "" {
			return parsedConfig.RemoteUser
		}
		if parsedConfig.ContainerUser != "" {
			return parsedConfig.ContainerUser
		}
	}

	if options.User != "" {
		return options.User
	}

	return "root"
}
