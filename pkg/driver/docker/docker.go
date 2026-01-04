package docker

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/user"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/pkg/compose"
	config2 "github.com/skevetter/devpod/pkg/config"
	"github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/skevetter/devpod/pkg/docker"
	"github.com/skevetter/devpod/pkg/driver"
	"github.com/skevetter/devpod/pkg/ide/jetbrains"
	provider2 "github.com/skevetter/devpod/pkg/provider"
	"github.com/skevetter/log"
)

func makeEnvironment(env map[string]string, log log.Logger) []string {
	if env == nil {
		return nil
	}

	ret := config.ObjectToList(env)
	if len(env) > 0 {
		log.WithFields(logrus.Fields{
			"variables": ret,
		}).Debug("using docker environment variables")
	}

	return ret
}

func NewDockerDriver(workspaceInfo *provider2.AgentWorkspaceInfo, log log.Logger) (driver.DockerDriver, error) {
	dockerCommand := "docker"
	if workspaceInfo.Agent.Docker.Path != "" {
		dockerCommand = workspaceInfo.Agent.Docker.Path
	}

	var builder docker.DockerBuilder
	var err error
	builder, err = docker.DockerBuilderFromString(workspaceInfo.Agent.Docker.Builder)
	if err != nil {
		return nil, err
	}

	log.WithFields(logrus.Fields{
		"command": dockerCommand,
	}).Debug("using docker command")
	return &dockerDriver{
		Docker: &docker.DockerHelper{
			DockerCommand: dockerCommand,
			Environment:   makeEnvironment(workspaceInfo.Agent.Docker.Env, log),
			ContainerID:   workspaceInfo.Workspace.Source.Container,
			Builder:       builder,
			Log:           log,
		},
		Log: log,
	}, nil
}

type dockerDriver struct {
	Docker  *docker.DockerHelper
	Compose *compose.ComposeHelper

	Log log.Logger
}

func (d *dockerDriver) TargetArchitecture(ctx context.Context, workspaceId string) (string, error) {
	return runtime.GOARCH, nil
}

func (d *dockerDriver) CommandDevContainer(ctx context.Context, workspaceId, user, command string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	container, err := d.FindDevContainer(ctx, workspaceId)
	if err != nil {
		return err
	} else if container == nil {
		return fmt.Errorf("container not found")
	}

	args := []string{"exec"}
	if stdin != nil {
		args = append(args, "-i")
	}
	args = append(args, "-u", user, container.ID, "sh", "-c", command)
	return d.Docker.Run(ctx, args, stdin, stdout, stderr)
}

func (d *dockerDriver) PushDevContainer(ctx context.Context, image string) error {
	// push image
	writer := d.Log.Writer(logrus.InfoLevel, false)
	defer func() { _ = writer.Close() }()

	// build args
	args := []string{
		"push",
		image,
	}

	// run command
	d.Log.WithFields(logrus.Fields{
		"command": d.Docker.DockerCommand,
		"args":    strings.Join(args, " "),
	}).Debug("running docker push command")
	err := d.Docker.Run(ctx, args, nil, writer, writer)
	if err != nil {
		return fmt.Errorf("push image %w", err)
	}

	return nil
}

func (d *dockerDriver) TagDevContainer(ctx context.Context, image, tag string) error {
	// Tag image
	writer := d.Log.Writer(logrus.InfoLevel, false)
	defer func() { _ = writer.Close() }()

	// build args
	args := []string{
		"tag",
		image,
		tag,
	}

	// run command
	d.Log.WithFields(logrus.Fields{
		"command": d.Docker.DockerCommand,
		"args":    strings.Join(args, " "),
	}).Debug("running docker tag command")
	err := d.Docker.Run(ctx, args, nil, writer, writer)
	if err != nil {
		return fmt.Errorf("tag image %w", err)
	}

	return nil
}

func (d *dockerDriver) DeleteDevContainer(ctx context.Context, workspaceId string) error {
	container, err := d.FindDevContainer(ctx, workspaceId)
	if err != nil {
		return err
	} else if container == nil {
		return nil
	}

	err = d.Docker.Remove(ctx, container.ID)
	if err != nil {
		return err
	}

	return nil
}

func (d *dockerDriver) StartDevContainer(ctx context.Context, workspaceId string) error {
	container, err := d.FindDevContainer(ctx, workspaceId)
	if err != nil {
		return err
	} else if container == nil {
		return fmt.Errorf("container not found")
	}

	return d.Docker.StartContainer(ctx, container.ID)
}

func (d *dockerDriver) StopDevContainer(ctx context.Context, workspaceId string) error {
	container, err := d.FindDevContainer(ctx, workspaceId)
	if err != nil {
		return err
	} else if container == nil {
		return fmt.Errorf("container not found")
	}

	return d.Docker.Stop(ctx, container.ID)
}

func (d *dockerDriver) InspectImage(ctx context.Context, imageName string) (*config.ImageDetails, error) {
	return d.Docker.InspectImage(ctx, imageName, true)
}

func (d *dockerDriver) GetImageTag(ctx context.Context, imageID string) (string, error) {
	return d.Docker.GetImageTag(ctx, imageID)
}

func (d *dockerDriver) ComposeHelper() (*compose.ComposeHelper, error) {
	if d.Compose != nil {
		return d.Compose, nil
	}

	var err error
	d.Compose, err = compose.NewComposeHelper(compose.DockerComposeCommand, d.Docker)
	return d.Compose, err
}

func (d *dockerDriver) DockerHelper() (*docker.DockerHelper, error) {
	if d.Docker == nil {
		return nil, fmt.Errorf("no docker helper available")
	}

	return d.Docker, nil
}

func (d *dockerDriver) FindDevContainer(ctx context.Context, workspaceId string) (*config.ContainerDetails, error) {
	var containerDetails *config.ContainerDetails
	var err error
	if d.Docker.ContainerID != "" {
		containerDetails, err = d.Docker.FindContainerByID(ctx, []string{d.Docker.ContainerID})
	} else {
		containerDetails, err = d.Docker.FindDevContainer(ctx, []string{config.DockerIDLabel + "=" + workspaceId})
	}
	if err != nil {
		return nil, err
	} else if containerDetails == nil {
		return nil, nil
	}

	if containerDetails.Config.User != "" {
		if containerDetails.Config.Labels == nil {
			containerDetails.Config.Labels = map[string]string{}
		}
		if containerDetails.Config.Labels[config.UserLabel] == "" {
			containerDetails.Config.Labels[config.UserLabel] = containerDetails.Config.User
		}
	}

	return containerDetails, nil
}

func (d *dockerDriver) RunDevContainer(
	ctx context.Context,
	workspaceId string,
	options *driver.RunOptions,
) error {
	return fmt.Errorf("unsupported")
}

func (d *dockerDriver) RunDockerDevContainer(
	ctx context.Context,
	workspaceId string,
	options *driver.RunOptions,
	parsedConfig *config.DevContainerConfig,
	init *bool,
	ide string,
	ideOptions map[string]config2.OptionValue,
) error {
	err := d.EnsureImage(ctx, options)
	if err != nil {
		return err
	}
	helper, err := d.DockerHelper()
	if err != nil {
		return err
	}

	args := []string{"run"}
	if !helper.IsNerdctl() {
		args = append(args, "--sig-proxy=false")
	}

	// add ports
	for _, appPort := range parsedConfig.AppPort {
		intPort, err := strconv.Atoi(appPort)
		if err != nil {
			args = append(args, "-p", appPort)
		} else {
			args = append(args, "-p", fmt.Sprintf("127.0.0.1:%d:%d", intPort, intPort))
		}
	}

	// workspace mount
	if options.WorkspaceMount != nil {
		workspacePath := d.EnsurePath(options.WorkspaceMount)
		mountPath := workspacePath.String()
		if helper.IsNerdctl() && strings.Contains(mountPath, ",consistency='consistent'") {
			mountPath = strings.Replace(mountPath, ",consistency='consistent'", "", 1)
		}

		args = append(args, "--mount", mountPath)
	}

	// override container user
	if options.User != "" {
		args = append(args, "-u", options.User)
	}

	// container env
	for k, v := range options.Env {
		args = append(args, "-e", k+"="+v)
	}

	if options.Privileged != nil && *options.Privileged {
		args = append(args, "--privileged")
	}

	// In case we're using podman, let's use userns to keep
	// the ID of the user (vscode) inside the container as
	// the same of the external user.
	// This will avoid problems of mismatching chowns on the
	// project files.
	if d.Docker.IsPodman() && os.Getuid() != 0 {
		args = append(args, "--userns", "keep-id")
	}

	for _, capAdd := range options.CapAdd {
		args = append(args, "--cap-add", capAdd)
	}
	for _, securityOpt := range options.SecurityOpt {
		args = append(args, "--security-opt", securityOpt)
	}

	for _, mount := range options.Mounts {
		if mount.Type == "bind" && mount.Source != "" {
			if _, err := os.Stat(mount.Source); os.IsNotExist(err) {
				return fmt.Errorf("bind mount source path does not exist %s", mount.Source)
			}
		}
		args = append(args, "--mount", mount.String())
	}

	// add ide mounts
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

	// labels
	labels := append(config.GetDockerLabelForID(workspaceId), options.Labels...)
	for _, label := range labels {
		args = append(args, "-l", label)
	}

	// check GPU
	if parsedConfig.HostRequirements != nil && parsedConfig.HostRequirements.GPU == "true" {
		enabled, _ := d.Docker.GPUSupportEnabled()
		if enabled {
			args = append(args, "--gpus", "all")
		}
	}

	// runArgs
	// check if we need to add --gpus=all to the run args based on the dev container's host requirments
	if parsedConfig.HostRequirements != nil {
		usesGpu, err := parsedConfig.HostRequirements.GPU.Bool()
		if err != nil && usesGpu {
			// check if the user manually add --gpus=all, if not then add it
			if !slices.Contains(parsedConfig.RunArgs, "--gpus=all") {
				args = append(args, "--gpus=all")
			}
		}
	}

	args = append(args, parsedConfig.RunArgs...)

	// run detached
	args = append(args, "-d")

	// add entrypoint
	if options.Entrypoint != "" {
		args = append(args, "--entrypoint", options.Entrypoint)
	}

	// image name
	args = append(args, options.Image)

	// entrypoint
	args = append(args, options.Cmd...)

	// run the command
	d.Log.WithFields(logrus.Fields{
		"command": d.Docker.DockerCommand,
		"args":    strings.Join(args, " "),
	}).Info("running docker command")
	writer := d.Log.Writer(logrus.InfoLevel, false)
	defer func() { _ = writer.Close() }()

	err = d.Docker.Run(ctx, args, nil, writer, writer)
	if err != nil {
		d.Log.WithFields(logrus.Fields{
			"error":   err,
			"command": d.Docker.DockerCommand,
			"args":    strings.Join(args, " "),
		}).Error("docker container failed to start")
		return fmt.Errorf("failed to start dev container %w", err)
	}

	if runtime.GOOS == "linux" {
		updateUID := parsedConfig.UpdateRemoteUserUID == nil || *parsedConfig.UpdateRemoteUserUID
		d.Log.WithFields(logrus.Fields{
			"updateRemoteUserUID": parsedConfig.UpdateRemoteUserUID,
			"willUpdateUID":       updateUID,
		}).Debug("UID update check")
		if updateUID {
			d.Log.WithFields(logrus.Fields{"workspaceId": workspaceId}).Debug("updating container user UID/GID")
			err = d.updateContainerUserUID(ctx, workspaceId, parsedConfig, options.WorkspaceMount, writer)
			if err != nil {
				d.Log.Errorf("failed to update container user UID/GID: %v", err)
				return fmt.Errorf("failed to update container user UID/GID: %w", err)
			}
		}
	}

	return nil
}

func (d *dockerDriver) EnsureImage(
	ctx context.Context,
	options *driver.RunOptions,
) error {
	d.Log.WithFields(logrus.Fields{
		"image": options.Image,
	}).Info("inspecting image")
	_, err := d.Docker.InspectImage(ctx, options.Image, false)
	if err != nil {
		d.Log.WithFields(logrus.Fields{
			"image": options.Image,
		}).Info("image not found")
		d.Log.WithFields(logrus.Fields{
			"image": options.Image,
		}).Info("pulling image")
		writer := d.Log.Writer(logrus.DebugLevel, false)
		defer func() { _ = writer.Close() }()

		return d.Docker.Pull(ctx, options.Image, nil, writer, writer)
	}
	return nil
}

func (d *dockerDriver) EnsurePath(path *config.Mount) *config.Mount {
	// in case of local windows and remote linux tcp, we need to manually do the path conversion
	if runtime.GOOS == "windows" {
		for _, v := range d.Docker.Environment {
			// we do this only is DOCKER_HOST is not docker-desktop engine, but
			// a direct TCP connection to a docker daemon running in WSL
			if strings.Contains(v, "DOCKER_HOST=tcp://") {
				unixPath := path.Source
				unixPath = strings.Replace(unixPath, "C:", "c", 1)
				unixPath = strings.ReplaceAll(unixPath, "\\", "/")
				unixPath = "/mnt/" + unixPath

				path.Source = unixPath

				return path
			}
		}
	}
	return path
}

func (d *dockerDriver) GetDevContainerLogs(ctx context.Context, workspaceId string, stdout io.Writer, stderr io.Writer) error {
	container, err := d.FindDevContainer(ctx, workspaceId)
	if err != nil {
		return err
	} else if container == nil {
		return fmt.Errorf("container not found")
	}

	return d.Docker.GetContainerLogs(ctx, container.ID, stdout, stderr)
}

func (d *dockerDriver) extractRemoteUserFromMetadata(metadata string) string {
	if !strings.Contains(metadata, `"remoteUser":"`) {
		return ""
	}

	start := strings.Index(metadata, `"remoteUser":"`) + len(`"remoteUser":"`)
	if start <= len(`"remoteUser":"`) {
		return ""
	}

	end := strings.Index(metadata[start:], `"`)
	if end <= 0 {
		return ""
	}

	return metadata[start : start+end]
}

// resolveContainerUser determines the user that should be used for UID/GID updates
// by following the priority order as described in the devcontainer specification.
//
// Priority order:
// 1. parsedConfig.RemoteUser - Explicit devcontainer.json remoteUser config
// 2. devcontainer metadata remoteUser - Parsed from container labels (e.g., "remoteUser":"vscode")
// 3. devpod.user label - DevPod's internal tracking label (fallback)
// 4. container.Config.User - Docker image default user (fallback)
// 5. parsedConfig.ContainerUser - Explicit devcontainer.json containerUser config (final fallback)
//
// This ensures that both UID updates and SSH sessions use the same user, preventing
// permission issues where UID update detects one user but SSH runs as another.
func (d *dockerDriver) resolveContainerUser(parsedConfig *config.DevContainerConfig, container *config.ContainerDetails) string {
	if parsedConfig.RemoteUser != "" {
		d.Log.Debugf("detected container user from RemoteUser config: %s", parsedConfig.RemoteUser)
		return parsedConfig.RemoteUser
	}

	if container.Config.Labels != nil {
		if metadata := container.Config.Labels["devcontainer.metadata"]; metadata != "" {
			if user := d.extractRemoteUserFromMetadata(metadata); user != "" {
				d.Log.Debugf("detected container user from devcontainer metadata: %s", user)
				return user
			}
		}
	}

	if container.Config.Labels != nil {
		if userLabel := container.Config.Labels[config.UserLabel]; userLabel != "" {
			d.Log.Debugf("detected container user from devpod label: %s", userLabel)
			return userLabel
		}
	}

	if container.Config.User != "" {
		userParts := strings.Split(container.Config.User, ":")
		if userParts[0] != "" {
			d.Log.Debugf("detected container user from docker Config.User: %s", userParts[0])
			return userParts[0]
		}
	}

	if parsedConfig.ContainerUser != "" {
		d.Log.Debugf("detected container user from ContainerUser config: %s", parsedConfig.ContainerUser)
		return parsedConfig.ContainerUser
	}

	return ""
}

func (d *dockerDriver) updateContainerUserUID(ctx context.Context, workspaceId string, parsedConfig *config.DevContainerConfig, workspaceMount *config.Mount, writer io.Writer) error {
	localUser, err := user.Current()
	if err != nil {
		return err
	}
	localUid := localUser.Uid
	localGid := localUser.Gid

	container, err := d.FindDevContainer(ctx, workspaceId)
	if err != nil {
		return err
	} else if container == nil {
		return nil
	}

	containerUser := d.resolveContainerUser(parsedConfig, container)
	d.Log.WithFields(logrus.Fields{
		"localUid":      localUid,
		"localGid":      localGid,
		"containerId":   container.ID,
		"containerUser": containerUser,
	}).Debug("preparing to update container user UID/GID")
	if containerUser == "" {
		d.Log.Debug("no container user specified, skipping UID/GID update")
		return nil
	}

	d.Log.Debug("creating temporary files for UID/GID update")
	containerPasswdFileIn, err := os.CreateTemp("", "devpod_container_passwd_in")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(containerPasswdFileIn.Name()) }()

	containerGroupFileIn, err := os.CreateTemp("", "devpod_container_group_in")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(containerGroupFileIn.Name()) }()

	containerPasswdFileOut, err := os.CreateTemp("", "devpod_container_passwd_out")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(containerPasswdFileOut.Name()) }()

	containerGroupFileOut, err := os.CreateTemp("", "devpod_container_group_out")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(containerGroupFileOut.Name()) }()

	args := []string{"cp", fmt.Sprintf("%s:/etc/passwd", container.ID), containerPasswdFileIn.Name()}
	d.Log.WithFields(logrus.Fields{"command": d.Docker.DockerCommand, "args": strings.Join(args, " ")}).Debug("running docker command")
	err = d.Docker.Run(ctx, args, nil, writer, writer)
	if err != nil {
		return err
	}

	args = []string{"cp", fmt.Sprintf("%s:/etc/group", container.ID), containerGroupFileIn.Name()}
	d.Log.WithFields(logrus.Fields{"command": d.Docker.DockerCommand, "args": strings.Join(args, " ")}).Debug("running docker command")
	err = d.Docker.Run(ctx, args, nil, writer, writer)
	if err != nil {
		return err
	}

	containerPasswdFileIn, err = os.Open(containerPasswdFileIn.Name())
	if err != nil {
		return err
	}
	defer func() { _ = containerPasswdFileIn.Close() }()

	scanner := bufio.NewScanner(containerPasswdFileIn)
	containerUid := ""
	containerGid := ""
	containerHome := ""

	re := regexp.MustCompile(fmt.Sprintf(`^%s:(?P<password>x?):(?P<uid>.*):(?P<gid>.*):(?P<gcos>.*):(?P<home>.*):(?P<shell>.*)$`, containerUser))
	for scanner.Scan() {
		match := re.FindStringSubmatch(scanner.Text())
		if match == nil {
			_, err := fmt.Fprintf(containerPasswdFileOut, "%s\n", scanner.Text())
			if err != nil {
				return err
			}
			continue
		}
		result := make(map[string]string)
		for i, name := range re.SubexpNames() {
			if i != 0 && name != "" {
				result[name] = match[i]
			}
		}
		containerUid = result["uid"]
		containerGid = result["gid"]
		containerHome = result["home"]

		d.Log.WithFields(logrus.Fields{
			"containerUid":  containerUid,
			"containerGid":  containerGid,
			"containerHome": containerHome,
		}).Debug("found container user details")
		_, err := fmt.Fprintf(containerPasswdFileOut, "%s:%s:%s:%s:%s:%s:%s\n", containerUser, result["password"], localUid, localGid, result["gcos"], result["home"], result["shell"])
		if err != nil {
			return err
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	if containerUid == "" {
		d.Log.WithFields(logrus.Fields{"containerUser": containerUser}).Debug("container user not found in /etc/passwd, skipping UID/GID update")
		return nil
	}

	if localUid == "0" || containerUid == "0" || (localUid == containerUid && localGid == containerGid) {
		d.Log.WithFields(logrus.Fields{
			"localUid":     localUid,
			"containerUid": containerUid,
			"localGid":     localGid,
			"containerGid": containerGid,
		}).Debug("no UID/GID update needed because user is root or uid/gid sources and targets are the same")
		return nil
	}

	containerGroupFileIn, err = os.Open(containerGroupFileIn.Name())
	if err != nil {
		return err
	}
	defer func() { _ = containerGroupFileIn.Close() }()

	scanner = bufio.NewScanner(containerGroupFileIn)

	re = regexp.MustCompile(fmt.Sprintf(`^(?P<group>.*):(?P<password>x?):%s:(?P<group_list>.*)$`, containerGid))
	for scanner.Scan() {
		match := re.FindStringSubmatch(scanner.Text())
		if match == nil {
			_, err := fmt.Fprintf(containerGroupFileOut, "%s\n", scanner.Text())
			if err != nil {
				return err
			}
			continue
		}
		result := make(map[string]string)
		for i, name := range re.SubexpNames() {
			if i != 0 && name != "" {
				result[name] = match[i]
			}
		}

		_, err := fmt.Fprintf(containerGroupFileOut, "%s:%s:%s:%s\n", result["group"], result["password"], localGid, result["group_list"])
		if err != nil {
			return err
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	d.Log.WithFields(logrus.Fields{
		"containerUser": containerUser,
		"containerUid":  containerUid,
		"containerGid":  containerGid,
		"localUid":      localUid,
		"localGid":      localGid,
	}).Info("updating container user UID and GID")

	// Copy /etc/passwd and /etc/group back to the container
	args = []string{"cp", containerPasswdFileOut.Name(), fmt.Sprintf("%s:/etc/passwd", container.ID)}
	d.Log.WithFields(logrus.Fields{"command": d.Docker.DockerCommand, "args": strings.Join(args, " ")}).Debug("running docker copy passwd command")
	err = d.Docker.Run(ctx, args, nil, writer, writer)
	if err != nil {
		return err
	}

	args = []string{"cp", containerGroupFileOut.Name(), fmt.Sprintf("%s:/etc/group", container.ID)}
	d.Log.WithFields(logrus.Fields{"command": d.Docker.DockerCommand, "args": strings.Join(args, " ")}).Debug("running docker copy group command")
	err = d.Docker.Run(ctx, args, nil, writer, writer)
	if err != nil {
		return err
	}

	args = []string{"exec", "-u", "root", container.ID, "chmod", "644", "/etc/passwd", "/etc/group"}
	d.Log.WithFields(logrus.Fields{"command": d.Docker.DockerCommand, "args": strings.Join(args, " ")}).Debug("running docker chmod command")
	err = d.Docker.Run(ctx, args, nil, writer, writer)
	if err != nil {
		return err
	}

	args = []string{"exec", "-u", "root", container.ID, "chown", "-R", fmt.Sprintf("%s:%s", localUid, localGid), containerHome}
	d.Log.WithFields(logrus.Fields{"command": d.Docker.DockerCommand, "args": strings.Join(args, " ")}).Debug("running docker chown command")
	err = d.Docker.Run(ctx, args, nil, writer, writer)
	if err != nil {
		return err
	}

	if workspaceMount != nil && workspaceMount.Target != "" {
		args = []string{"exec", "-u", "root", container.ID, "chown", fmt.Sprintf("%s:%s", localUid, localGid), workspaceMount.Target}
		d.Log.WithFields(logrus.Fields{"command": d.Docker.DockerCommand, "args": strings.Join(args, " ")}).Debug("running docker chown workspace parent command")
		err = d.Docker.Run(ctx, args, nil, writer, writer)
		if err != nil {
			d.Log.WithFields(logrus.Fields{"error": err, "command": d.Docker.DockerCommand, "args": strings.Join(args, " ")}).Warn("failed to chown workspace parent directory")
		}
	}

	return nil
}
