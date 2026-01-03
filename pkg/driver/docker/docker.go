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

	return d.UpdateContainerUserUID(ctx, workspaceId, parsedConfig)
}

func (d *dockerDriver) UpdateContainerUserUID(ctx context.Context, workspaceId string, parsedConfig *config.DevContainerConfig) error {
	d.Log.WithFields(logrus.Fields{
		"runtimeGOOS":         runtime.GOOS,
		"containerUser":       parsedConfig.ContainerUser,
		"remoteUser":          parsedConfig.RemoteUser,
		"updateRemoteUserUID": parsedConfig.UpdateRemoteUserUID,
	}).Info("updating container UID/GID")

	if !d.shouldUpdateUID(parsedConfig) {
		return nil
	}

	localUser, err := user.Current()
	if err != nil {
		return err
	}

	container, err := d.findAndValidateContainer(ctx, workspaceId)
	if err != nil {
		return err
	}

	containerDetails, err := d.Docker.InspectContainers(ctx, []string{container.ID})
	if err != nil {
		d.Log.WithFields(logrus.Fields{
			"error":       err,
			"containerId": container.ID,
		}).Error("failed to inspect container")
		return err
	}

	var containerDetail *config.ContainerDetails
	if len(containerDetails) > 0 {
		containerDetail = &containerDetails[0]
	}

	result := &config.Result{
		MergedConfig:     &config.MergedDevContainerConfig{},
		ContainerDetails: containerDetail,
	}
	if parsedConfig.ContainerUser != "" {
		result.MergedConfig.ContainerUser = parsedConfig.ContainerUser
	}
	if parsedConfig.RemoteUser != "" {
		result.MergedConfig.RemoteUser = parsedConfig.RemoteUser
	}
	containerUser := config.GetRemoteUser(result)
	if containerUser == "" {
		d.Log.Debug("no container user found, skipping UID/GID mapping")
		return nil
	}

	if containerUser == "root" {
		// root user needs UID 0 for system operations like agent injection
		d.Log.Debug("skipping UID/GID mapping for root user to preserve system permissions")
		return nil
	}

	return d.updateContainerUserFiles(ctx, container, containerUser, localUser, parsedConfig)
}

func (d *dockerDriver) shouldUpdateUID(parsedConfig *config.DevContainerConfig) bool {
	if runtime.GOOS != "linux" {
		d.Log.Info("os is not linux; skipping UID/GID mapping")
		return false
	}

	isUpdateRemoteUserUIDDisabled := parsedConfig.UpdateRemoteUserUID != nil && !*parsedConfig.UpdateRemoteUserUID
	if isUpdateRemoteUserUIDDisabled {
		d.Log.Info("updateRemoteUserUID is disabled; skipping UID/GID mapping")
		return false
	}

	return true
}

func (d *dockerDriver) findAndValidateContainer(ctx context.Context, workspaceId string) (*config.ContainerDetails, error) {
	container, err := d.FindDevContainer(ctx, workspaceId)
	if err != nil {
		d.Log.WithFields(logrus.Fields{
			"error":       err,
			"workspaceId": workspaceId,
		}).Error("failed to find container")
		return nil, err
	}

	if container == nil {
		d.Log.WithFields(logrus.Fields{
			"workspaceId": workspaceId,
		}).Info("no container found for workspace")
		return nil, fmt.Errorf("no container found for workspace %s", workspaceId)
	}

	d.Log.WithFields(logrus.Fields{
		"containerId": container.ID,
	}).Debug("found container")

	return container, nil
}

func (d *dockerDriver) updateContainerUserFiles(ctx context.Context, container *config.ContainerDetails, containerUser string, localUser *user.User, parsedConfig *config.DevContainerConfig) error {
	tempFiles, err := d.createTempFiles()
	if err != nil {
		return err
	}
	defer d.cleanupTempFiles(tempFiles)

	writer := d.Log.Writer(logrus.InfoLevel, false)
	defer func() { _ = writer.Close() }()

	err = d.copyContainerFiles(ctx, container, tempFiles, writer)
	if err != nil {
		return err
	}

	userInfo, err := d.processPasswdFile(tempFiles, containerUser, localUser)
	if err != nil {
		return err
	}

	if localUser.Uid == userInfo.containerUid && localUser.Gid == userInfo.containerGid {
		d.Log.WithFields(logrus.Fields{
			"localUid":     localUser.Uid,
			"containerUid": userInfo.containerUid,
			"localGid":     localUser.Gid,
			"containerGid": userInfo.containerGid,
		}).Info("no UID/GID mapping needed")
		return nil
	}

	err = d.processGroupFile(tempFiles, userInfo.containerGid, localUser.Gid)
	if err != nil {
		return err
	}

	return d.updateContainerFilesAndPermissions(ctx, container, tempFiles, userInfo, localUser, parsedConfig, writer)
}

type containerUserInfo struct {
	containerUid  string
	containerGid  string
	containerHome string
}

type tempFiles struct {
	passwdIn  *os.File
	groupIn   *os.File
	passwdOut *os.File
	groupOut  *os.File
}

func (d *dockerDriver) createTempFiles() (*tempFiles, error) {
	passwdIn, err := os.CreateTemp("", "devpod_container_passwd_in")
	if err != nil {
		return nil, err
	}

	groupIn, err := os.CreateTemp("", "devpod_container_group_in")
	if err != nil {
		os.Remove(passwdIn.Name())
		return nil, err
	}

	passwdOut, err := os.CreateTemp("", "devpod_container_passwd_out")
	if err != nil {
		os.Remove(passwdIn.Name())
		os.Remove(groupIn.Name())
		return nil, err
	}

	groupOut, err := os.CreateTemp("", "devpod_container_group_out")
	if err != nil {
		os.Remove(passwdIn.Name())
		os.Remove(groupIn.Name())
		os.Remove(passwdOut.Name())
		return nil, err
	}

	return &tempFiles{
		passwdIn:  passwdIn,
		groupIn:   groupIn,
		passwdOut: passwdOut,
		groupOut:  groupOut,
	}, nil
}

func (d *dockerDriver) cleanupTempFiles(files *tempFiles) {
	if files != nil {
		os.Remove(files.passwdIn.Name())
		os.Remove(files.groupIn.Name())
		os.Remove(files.passwdOut.Name())
		os.Remove(files.groupOut.Name())
	}
}

func (d *dockerDriver) copyContainerFiles(ctx context.Context, container *config.ContainerDetails, files *tempFiles, writer io.Writer) error {
	args := []string{"cp", fmt.Sprintf("%s:/etc/passwd", container.ID), files.passwdIn.Name()}
	d.Log.WithFields(logrus.Fields{
		"command": d.Docker.DockerCommand,
		"args":    strings.Join(args, " "),
	}).Debug("copying container passwd file")
	err := d.Docker.Run(ctx, args, nil, writer, writer)
	if err != nil {
		return err
	}

	args = []string{"cp", fmt.Sprintf("%s:/etc/group", container.ID), files.groupIn.Name()}
	d.Log.WithFields(logrus.Fields{
		"command": d.Docker.DockerCommand,
		"args":    strings.Join(args, " "),
	}).Debug("copying container group file")
	return d.Docker.Run(ctx, args, nil, writer, writer)
}

func (d *dockerDriver) processPasswdFile(files *tempFiles, containerUser string, localUser *user.User) (*containerUserInfo, error) {
	passwdFile, err := os.Open(files.passwdIn.Name())
	if err != nil {
		return nil, err
	}
	defer passwdFile.Close()

	scanner := bufio.NewScanner(passwdFile)
	userInfo := &containerUserInfo{}

	re := regexp.MustCompile(fmt.Sprintf(`^%s:(?P<password>x?):(?P<uid>.*):(?P<gid>.*):(?P<gcos>.*):(?P<home>.*):(?P<shell>.*)$`, containerUser))
	d.Log.WithFields(logrus.Fields{
		"containerUser": containerUser,
	}).Debug("scanning container passwd file for user")

	for scanner.Scan() {
		line := scanner.Text()
		match := re.FindStringSubmatch(line)
		if match == nil {
			_, err := fmt.Fprintf(files.passwdOut, "%s\n", line)
			if err != nil {
				return nil, err
			}
			continue
		}

		d.Log.Debug("found user in passwd file")
		result := make(map[string]string)
		for i, name := range re.SubexpNames() {
			if i != 0 && name != "" {
				result[name] = match[i]
			}
		}

		userInfo.containerUid = result["uid"]
		userInfo.containerGid = result["gid"]
		userInfo.containerHome = result["home"]

		_, err := fmt.Fprintf(files.passwdOut, "%s:%s:%s:%s:%s:%s:%s\n",
			containerUser, result["password"], localUser.Uid, localUser.Gid,
			result["gcos"], result["home"], result["shell"])
		if err != nil {
			return nil, err
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if userInfo.containerUid == "" || userInfo.containerGid == "" || userInfo.containerHome == "" {
		d.Log.WithFields(logrus.Fields{
			"containerUser": containerUser,
			"containerUid":  userInfo.containerUid,
			"containerGid":  userInfo.containerGid,
			"containerHome": userInfo.containerHome,
		}).Error("user lookup validation failed")
		return nil, fmt.Errorf("user %q not found in container /etc/passwd", containerUser)
	}

	return userInfo, nil
}

func (d *dockerDriver) processGroupFile(files *tempFiles, containerGid, localGid string) error {
	groupFile, err := os.Open(files.groupIn.Name())
	if err != nil {
		return err
	}
	defer groupFile.Close()

	scanner := bufio.NewScanner(groupFile)
	re := regexp.MustCompile(fmt.Sprintf(`^(?P<group>.*):(?P<password>x?):%s:(?P<group_list>.*)$`, containerGid))

	for scanner.Scan() {
		match := re.FindStringSubmatch(scanner.Text())
		if match == nil {
			_, err := fmt.Fprintf(files.groupOut, "%s\n", scanner.Text())
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

		_, err := fmt.Fprintf(files.groupOut, "%s:%s:%s:%s\n",
			result["group"], result["password"], localGid, result["group_list"])
		if err != nil {
			return err
		}
	}

	return scanner.Err()
}

func (d *dockerDriver) updateContainerFilesAndPermissions(ctx context.Context, container *config.ContainerDetails, files *tempFiles, userInfo *containerUserInfo, localUser *user.User, parsedConfig *config.DevContainerConfig, writer io.Writer) error {
	d.Log.WithFields(logrus.Fields{
		"containerUser": parsedConfig.ContainerUser,
		"containerUid":  userInfo.containerUid,
		"containerGid":  userInfo.containerGid,
		"localUid":      localUser.Uid,
		"localGid":      localUser.Gid,
	}).Info("updating container user UID and GID")

	args := []string{"cp", files.passwdOut.Name(), fmt.Sprintf("%s:/etc/passwd", container.ID)}
	d.Log.WithFields(logrus.Fields{
		"command": d.Docker.DockerCommand,
		"args":    strings.Join(args, " "),
	}).Debug("copying container passwd file")
	err := d.Docker.Run(ctx, args, nil, writer, writer)
	if err != nil {
		return err
	}

	args = []string{"cp", files.groupOut.Name(), fmt.Sprintf("%s:/etc/group", container.ID)}
	d.Log.WithFields(logrus.Fields{
		"command": d.Docker.DockerCommand,
		"args":    strings.Join(args, " "),
	}).Debug("copying container group file")
	err = d.Docker.Run(ctx, args, nil, writer, writer)
	if err != nil {
		return err
	}

	args = []string{"exec", "-u", "root", container.ID, "chmod", "644", "/etc/passwd", "/etc/group"}
	d.Log.WithFields(logrus.Fields{
		"command": d.Docker.DockerCommand,
		"args":    strings.Join(args, " "),
	}).Debug("modifying container passwd and group files permissions")
	err = d.Docker.Run(ctx, args, nil, writer, writer)
	if err != nil {
		return err
	}

	err = d.updateHomeDirectoryOwnership(ctx, container, userInfo.containerHome, localUser, writer)
	if err != nil {
		return err
	}

	return d.updateWorkspaceOwnership(ctx, container, userInfo.containerHome, localUser, parsedConfig, writer)
}

func (d *dockerDriver) updateHomeDirectoryOwnership(ctx context.Context, container *config.ContainerDetails, containerHome string, localUser *user.User, writer io.Writer) error {
	if containerHome == "" || containerHome == "/" || strings.Contains(containerHome, "..") {
		return fmt.Errorf("invalid container home directory %s", containerHome)
	}

	if containerHome == "/root" && os.Geteuid() != 0 {
		d.Log.WithFields(logrus.Fields{
			"containerHome": containerHome,
			"localUid":      localUser.Uid,
			"localGid":      localUser.Gid,
		}).Debug("skipping chown of /root directory when running as non-root user")
		return nil
	}

	args := []string{"exec", "-u", "root", container.ID, "chown", "-R", fmt.Sprintf("%s:%s", localUser.Uid, localUser.Gid), containerHome}
	d.Log.WithFields(logrus.Fields{
		"command": d.Docker.DockerCommand,
		"args":    strings.Join(args, " "),
	}).Debug("changing ownership of container home directory")

	return d.Docker.Run(ctx, args, nil, writer, writer)
}

func (d *dockerDriver) updateWorkspaceOwnership(ctx context.Context, container *config.ContainerDetails, containerHome string, localUser *user.User, parsedConfig *config.DevContainerConfig, writer io.Writer) error {
	normalizedWorkspaceFolder := strings.TrimRight(parsedConfig.WorkspaceFolder, "/")
	normalizedContainerHome := strings.TrimRight(containerHome, "/")

	if normalizedWorkspaceFolder == "" || normalizedWorkspaceFolder == normalizedContainerHome {
		d.Log.Debug("skipping workspace chown")
		return nil
	}

	// Validate workspace folder path for security
	if strings.Contains(parsedConfig.WorkspaceFolder, "..") || !strings.HasPrefix(parsedConfig.WorkspaceFolder, "/") {
		d.Log.WithFields(logrus.Fields{
			"workspaceFolder": parsedConfig.WorkspaceFolder,
			"hasDoubleDot":    strings.Contains(parsedConfig.WorkspaceFolder, ".."),
			"hasAbsolutePath": strings.HasPrefix(parsedConfig.WorkspaceFolder, "/"),
		}).Error("workspace folder path validation failed")
		return fmt.Errorf("invalid workspace folder path: %s", parsedConfig.WorkspaceFolder)
	}

	args := []string{"exec", "-u", "root", container.ID, "chown", "-R", fmt.Sprintf("%s:%s", localUser.Uid, localUser.Gid), parsedConfig.WorkspaceFolder}
	d.Log.WithFields(logrus.Fields{
		"command": d.Docker.DockerCommand,
		"args":    strings.Join(args, " "),
	}).Debug("changing ownership of workspace folder")

	return d.Docker.Run(ctx, args, nil, writer, writer)
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
