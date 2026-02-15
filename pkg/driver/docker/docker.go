package docker

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/user"
	"runtime"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/pkg/compose"
	"github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/skevetter/devpod/pkg/docker"
	"github.com/skevetter/devpod/pkg/driver"
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

	log.WithFields(logrus.Fields{"command": dockerCommand}).Debug("using docker command")
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
		return fmt.Errorf("push image: %w", err)
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
		return fmt.Errorf("tag image: %w", err)
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
	d.Compose, err = compose.NewComposeHelper(d.Docker)
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

func (d *dockerDriver) RunDockerDevContainer(ctx context.Context, params *driver.RunDockerDevContainerParams) error {
	if err := d.EnsureImage(ctx, params.Options); err != nil {
		return err
	}

	helper, err := d.DockerHelper()
	if err != nil {
		return err
	}

	args, err := d.buildRunArgs(params, helper)
	if err != nil {
		return err
	}

	writer := d.Log.Writer(logrus.InfoLevel, false)
	defer func() { _ = writer.Close() }()

	if err := d.startContainer(ctx, args, writer); err != nil {
		return err
	}

	return d.UpdateContainerUserUID(ctx, params.WorkspaceID, params.ParsedConfig, writer)
}

func (d *dockerDriver) EnsureImage(
	ctx context.Context,
	options *driver.RunOptions,
) error {
	d.Log.WithFields(logrus.Fields{"image": options.Image}).Info("inspecting image")
	_, err := d.Docker.InspectImage(ctx, options.Image, false)
	if err != nil {
		d.Log.WithFields(logrus.Fields{"image": options.Image}).Info("image not found, pulling image")
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

func (d *dockerDriver) shouldUpdateUserUID(parsedConfig *config.DevContainerConfig) bool {
	isLinux := runtime.GOOS == "linux"
	hasUser := parsedConfig.ContainerUser != "" || parsedConfig.RemoteUser != ""
	shouldUpdate := parsedConfig.UpdateRemoteUserUID == nil || *parsedConfig.UpdateRemoteUserUID
	return isLinux && hasUser && shouldUpdate
}

func (d *dockerDriver) getContainerUser(parsedConfig *config.DevContainerConfig) string {
	if parsedConfig.RemoteUser != "" {
		return parsedConfig.RemoteUser
	}
	return parsedConfig.ContainerUser
}

func (d *dockerDriver) copyFileFromContainer(ctx context.Context, containerID, srcPath, dstPath string, writer io.Writer) error {
	args := []string{"cp", fmt.Sprintf("%s:%s", containerID, srcPath), dstPath}
	d.Log.WithFields(logrus.Fields{"command": d.Docker.DockerCommand, "args": strings.Join(args, " ")}).Debug("copying file from container")
	return d.Docker.Run(ctx, args, nil, writer, writer)
}

func (d *dockerDriver) copyFileToContainer(ctx context.Context, srcPath, containerID, dstPath string, writer io.Writer) error {
	args := []string{"cp", srcPath, fmt.Sprintf("%s:%s", containerID, dstPath)}
	d.Log.WithFields(logrus.Fields{"command": d.Docker.DockerCommand, "args": strings.Join(args, " ")}).Debug("copying file to container")
	return d.Docker.Run(ctx, args, nil, writer, writer)
}

type userInfo struct {
	uid  string
	gid  string
	home string
}

type lineProcessor func(line string, fields []string) (modifiedLine string, shouldWrite bool, err error)

func (d *dockerDriver) processColonDelimitedFile(in *os.File, out *os.File, fieldCount int, processor lineProcessor) error {
	scanner := bufio.NewScanner(in)

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.SplitN(line, ":", fieldCount)

		if len(fields) < fieldCount {
			if _, err := fmt.Fprintf(out, "%s\n", line); err != nil {
				return err
			}
			continue
		}

		modifiedLine, shouldWrite, err := processor(line, fields)
		if err != nil {
			return err
		}

		if shouldWrite {
			if _, err := fmt.Fprintf(out, "%s\n", modifiedLine); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprintf(out, "%s\n", line); err != nil {
				return err
			}
		}
	}

	return scanner.Err()
}

// updatePasswdFile processes /etc/passwd, replacing the target user's UID/GID with local values.
// It reads each line from passwdIn, and for lines matching containerUser, extracts the original
// UID, GID, and home directory, then writes a modified entry with localUid and localGid to passwdOut.
// All other lines are copied unchanged. Returns userInfo with the original container values, or an
// error if the user is not found in the passwd file.
func (d *dockerDriver) updatePasswdFile(passwdIn *os.File, passwdOut *os.File, containerUser, localUid, localGid string) (*userInfo, error) {
	info := &userInfo{}

	// parse passwd format: username:password:uid:gid:gecos:home:shell
	processor := func(line string, fields []string) (string, bool, error) {
		if fields[0] != containerUser {
			return "", false, nil
		}

		info.uid = fields[2]
		info.gid = fields[3]
		info.home = fields[5]

		modifiedLine := strings.Join([]string{fields[0], fields[1], localUid, localGid, fields[4], fields[5], fields[6]}, ":")
		return modifiedLine, true, nil
	}

	if err := d.processColonDelimitedFile(passwdIn, passwdOut, 7, processor); err != nil {
		return nil, err
	}

	if info.uid == "" {
		return nil, fmt.Errorf("user %q not found in passwd", containerUser)
	}

	return info, nil
}

// updateGroupFile processes /etc/group, replacing entries with the target GID to use localGid.
// It reads each line from groupIn, and for lines where the GID field matches containerGid,
// writes a modified entry with localGid to groupOut. All other lines are copied unchanged.
// Returns an error if scanning fails.
func (d *dockerDriver) updateGroupFile(groupIn *os.File, groupOut *os.File, containerGid, localGid string) error {
	// parse group format: groupname:password:gid:user_list
	processor := func(line string, fields []string) (string, bool, error) {
		if fields[2] != containerGid {
			return "", false, nil
		}

		modifiedLine := strings.Join([]string{fields[0], fields[1], localGid, fields[3]}, ":")
		return modifiedLine, true, nil
	}

	return d.processColonDelimitedFile(groupIn, groupOut, 4, processor)
}

func (d *dockerDriver) applyPermissions(ctx context.Context, containerID, localUid, localGid, containerHome string, writer io.Writer) error {
	args := []string{"exec", "-u", "root", containerID, "chmod", "644", "/etc/passwd", "/etc/group"}
	d.Log.WithFields(logrus.Fields{"command": d.Docker.DockerCommand, "args": strings.Join(args, " ")}).Debug("modifying permissions of /etc/passwd and /etc/group")
	if err := d.Docker.Run(ctx, args, nil, writer, writer); err != nil {
		return err
	}

	if containerHome == "" {
		d.Log.WithFields(logrus.Fields{"containerID": containerID}).Warn("container home directory not found, skipping chown")
		return nil
	}

	args = []string{"exec", "-u", "root", containerID, "chown", "-R", fmt.Sprintf("%s:%s", localUid, localGid), containerHome}
	d.Log.WithFields(logrus.Fields{"command": d.Docker.DockerCommand, "args": strings.Join(args, " ")}).Debug("running docker chown command")
	return d.Docker.Run(ctx, args, nil, writer, writer)
}

type tempFiles struct {
	passwdIn  *os.File
	groupIn   *os.File
	passwdOut *os.File
	groupOut  *os.File
}

func (t *tempFiles) cleanup() {
	if t.passwdIn != nil {
		_ = t.passwdIn.Close()
		_ = os.Remove(t.passwdIn.Name())
	}
	if t.groupIn != nil {
		_ = t.groupIn.Close()
		_ = os.Remove(t.groupIn.Name())
	}
	if t.passwdOut != nil {
		_ = t.passwdOut.Close()
		_ = os.Remove(t.passwdOut.Name())
	}
	if t.groupOut != nil {
		_ = t.groupOut.Close()
		_ = os.Remove(t.groupOut.Name())
	}
}

func (d *dockerDriver) createTempFiles() (*tempFiles, error) {
	files := &tempFiles{}
	var err error

	files.passwdIn, err = os.CreateTemp("", "devpod_container_passwd_in")
	if err != nil {
		return nil, err
	}

	files.groupIn, err = os.CreateTemp("", "devpod_container_group_in")
	if err != nil {
		files.cleanup()
		return nil, err
	}

	files.passwdOut, err = os.CreateTemp("", "devpod_container_passwd_out")
	if err != nil {
		files.cleanup()
		return nil, err
	}

	files.groupOut, err = os.CreateTemp("", "devpod_container_group_out")
	if err != nil {
		files.cleanup()
		return nil, err
	}

	return files, nil
}

func (d *dockerDriver) fetchContainerFiles(ctx context.Context, containerID string, files *tempFiles, writer io.Writer) error {
	if err := d.copyFileFromContainer(ctx, containerID, "/etc/passwd", files.passwdIn.Name(), writer); err != nil {
		return err
	}
	return d.copyFileFromContainer(ctx, containerID, "/etc/group", files.groupIn.Name(), writer)
}

func (d *dockerDriver) processUserFiles(files *tempFiles, containerUser, localUid, localGid string) (*userInfo, error) {
	passwdIn, err := os.Open(files.passwdIn.Name())
	if err != nil {
		return nil, err
	}
	defer func() { _ = passwdIn.Close() }()

	info, err := d.updatePasswdFile(passwdIn, files.passwdOut, containerUser, localUid, localGid)
	if err != nil {
		return nil, err
	}

	groupIn, err := os.Open(files.groupIn.Name())
	if err != nil {
		return nil, err
	}
	defer func() { _ = groupIn.Close() }()

	return info, d.updateGroupFile(groupIn, files.groupOut, info.gid, localGid)
}

func (d *dockerDriver) uploadUpdatedFiles(ctx context.Context, containerID string, files *tempFiles, writer io.Writer) error {
	if err := d.copyFileToContainer(ctx, files.passwdOut.Name(), containerID, "/etc/passwd", writer); err != nil {
		return err
	}
	return d.copyFileToContainer(ctx, files.groupOut.Name(), containerID, "/etc/group", writer)
}

func (d *dockerDriver) UpdateContainerUserUID(ctx context.Context, workspaceId string, parsedConfig *config.DevContainerConfig, writer io.Writer) error {
	if !d.shouldUpdateUserUID(parsedConfig) {
		return nil
	}

	localUser, err := user.Current()
	if err != nil {
		return err
	}

	containerUser := d.getContainerUser(parsedConfig)
	if containerUser == "" {
		return nil
	}

	container, err := d.FindDevContainer(ctx, workspaceId)
	if err != nil || container == nil {
		return err
	}

	files, err := d.createTempFiles()
	if err != nil {
		return err
	}
	defer files.cleanup()

	if err := d.fetchContainerFiles(ctx, container.ID, files, writer); err != nil {
		return err
	}

	info, err := d.processUserFiles(files, containerUser, localUser.Uid, localUser.Gid)
	if err != nil {
		return err
	}

	if localUser.Uid == "0" || info.uid == "0" || (localUser.Uid == info.uid && localUser.Gid == info.gid) {
		return nil
	}

	d.Log.WithFields(logrus.Fields{
		"containerUser": containerUser,
		"containerUid":  info.uid,
		"containerGid":  info.gid,
		"localUid":      localUser.Uid,
		"localGid":      localUser.Gid,
	}).Info("updating container user UID and GID")

	if err := d.uploadUpdatedFiles(ctx, container.ID, files, writer); err != nil {
		return err
	}

	return d.applyPermissions(ctx, container.ID, localUser.Uid, localUser.Gid, info.home, writer)
}
