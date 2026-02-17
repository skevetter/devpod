package pro

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/mgutz/ansi"
	"github.com/sirupsen/logrus"
	"github.com/skevetter/log"
	"github.com/skevetter/log/hash"
	"github.com/skevetter/log/scanner"
	"k8s.io/apimachinery/pkg/util/wait"
)

func (cmd *StartCmd) startDocker(ctx context.Context) error {
	cmd.Log.Infof("Starting DevPod Pro in Docker...")
	name := "devpod-pro"

	// prepare installation
	err := cmd.prepareDocker()
	if err != nil {
		return err
	}

	// try to find loft container
	containerID, err := cmd.findLoftContainer(ctx, name, true)
	if err != nil {
		return err
	}

	// check if container is there
	if containerID != "" && (cmd.Reset || cmd.Upgrade) {
		cmd.Log.Info("Existing instance found.")
		err = cmd.uninstallDocker(ctx, containerID)
		if err != nil {
			return err
		}

		containerID = ""
	}

	// Use default password if none is set
	if cmd.Password == "" {
		cmd.Password = getMachineUID(cmd.Log)
	}

	// check if is installed
	if containerID != "" {
		cmd.Log.Info("Existing instance found. Run with --upgrade to apply new configuration")
		return cmd.successDocker(ctx, containerID)
	}

	// Install Loft
	cmd.Log.Info("Welcome to DevPod Pro!")
	cmd.Log.Info("This installer will help you get started.")

	// make sure we are ready for installing
	containerID, err = cmd.runInDocker(ctx, name)
	if err != nil {
		return err
	} else if containerID == "" {
		return fmt.Errorf("%w: %s", ErrMissingContainer, "couldn't find Loft container after starting it")
	}

	return cmd.successDocker(ctx, containerID)
}

func (cmd *StartCmd) successDocker(ctx context.Context, containerID string) error {
	if cmd.NoWait {
		return nil
	}

	// wait until Loft is ready
	host, err := cmd.waitForLoftDocker(ctx, containerID)
	if err != nil {
		return err
	}

	// wait for domain to become reachable
	cmd.Log.Infof("Wait for DevPod Pro to become available at %s...", host)
	err = wait.PollUntilContextTimeout(ctx, time.Second, time.Minute*10, true, func(ctx context.Context) (bool, error) {
		containerDetails, err := cmd.inspectContainer(ctx, containerID)
		if err != nil {
			return false, fmt.Errorf("inspect loft container: %w", err)
		} else if strings.ToLower(containerDetails.State.Status) == "exited" || strings.ToLower(containerDetails.State.Status) == "dead" {
			logs, _ := cmd.logsContainer(ctx, containerID)
			return false, fmt.Errorf("container failed (status: %s):\n %s", containerDetails.State.Status, logs)
		}

		return isHostReachable(ctx, host)
	})
	if err != nil {
		return fmt.Errorf("error waiting for DevPod Pro: %w", err)
	}

	// print success message
	PrintSuccessMessageDockerInstall(host, cmd.Password, cmd.Log)
	return nil
}

func PrintSuccessMessageDockerInstall(host, password string, log log.Logger) {
	url := "https://" + host
	log.WriteString(logrus.InfoLevel, fmt.Sprintf(`


##########################   LOGIN   ############################

Username: `+ansi.Color("admin", "green+b")+`
Password: `+ansi.Color(password, "green+b")+`

Login via UI:  %s
Login via CLI: %s

#################################################################

DevPod Pro was successfully installed and can now be reached at: %s

Thanks for using DevPod Pro!
`,
		ansi.Color(url, "green+b"),
		ansi.Color("devpod pro login"+" "+url, "green+b"),
		url,
	))
}

func (cmd *StartCmd) waitForLoftDocker(ctx context.Context, containerID string) (string, error) {
	cmd.Log.Info("Wait for DevPod Pro to become available...")

	// check for local port
	containerDetails, err := cmd.inspectContainer(ctx, containerID)
	if err != nil {
		return "", err
	} else if len(containerDetails.NetworkSettings.Ports) > 0 && len(containerDetails.NetworkSettings.Ports["10443/tcp"]) > 0 {
		return "localhost:" + containerDetails.NetworkSettings.Ports["10443/tcp"][0].HostPort, nil
	}

	// check if no tunnel
	if cmd.NoTunnel {
		return "", fmt.Errorf("%w: %s", ErrLoftNotReachable, "cannot connect to DevPod Pro as it has no exposed port and --no-tunnel is enabled")
	}

	// wait for router
	url := ""
	waitErr := wait.PollUntilContextTimeout(ctx, time.Second, time.Minute*10, true, func(ctx context.Context) (bool, error) {
		url, err = cmd.findLoftRouter(ctx, containerID)
		if err != nil {
			return false, nil
		}

		return true, nil
	})
	if waitErr != nil {
		return "", fmt.Errorf("error waiting for loft router domain: %w", err)
	}

	return url, nil
}

func (cmd *StartCmd) findLoftRouter(ctx context.Context, id string) (string, error) {
	out, err := cmd.buildDockerCmd(ctx, "exec", id, "cat", "/var/lib/loft/loft-domain.txt").Output()
	if err != nil {
		return "", WrapCommandError(out, err)
	}

	return strings.TrimSpace(string(out)), nil
}

func (cmd *StartCmd) prepareDocker() error {
	// test for helm and kubectl
	_, err := exec.LookPath("docker")
	if err != nil {
		return fmt.Errorf("seems like docker is not installed. Docker is required for the installation of loft. Please visit https://docs.docker.com/engine/install/ for install instructions")
	}

	output, err := exec.Command("docker", "ps").CombinedOutput()
	if err != nil {
		return fmt.Errorf("seems like there are issues with your docker cli: \n\n%s", output)
	}

	return nil
}

func (cmd *StartCmd) uninstallDocker(ctx context.Context, id string) error {
	cmd.Log.Infof("Uninstalling...")

	// stop container
	out, err := cmd.buildDockerCmd(ctx, "stop", id).Output()
	if err != nil {
		return fmt.Errorf("stop container: %w", WrapCommandError(out, err))
	}

	// remove container
	out, err = cmd.buildDockerCmd(ctx, "rm", id).Output()
	if err != nil {
		return fmt.Errorf("remove container: %w", WrapCommandError(out, err))
	}

	return nil
}

func (cmd *StartCmd) runInDocker(ctx context.Context, name string) (string, error) {
	args := []string{"run", "-d", "--name", name}
	if cmd.NoTunnel {
		args = append(args, "--env", "DISABLE_LOFT_ROUTER=true")
	}
	if cmd.Password != "" {
		args = append(args, "--env", "ADMIN_PASSWORD_HASH="+hash.String(cmd.Password))
	}

	// run as root otherwise we get permission errors
	args = append(args, "-u", "root")

	// mount the loft lib
	args = append(args, "-v", "loft-data:/var/lib/loft")

	// set port
	if cmd.LocalPort != "" {
		args = append(args, "-p", cmd.LocalPort+":10443")
	}

	// set extra args
	args = append(args, cmd.DockerArgs...)

	// set image
	if cmd.DockerImage != "" {
		args = append(args, cmd.DockerImage)
	} else if cmd.Version != "" {
		args = append(args, "ghcr.io/loft-sh/devpod-pro:"+strings.TrimPrefix(cmd.Version, "v"))
	} else {
		args = append(args, "ghcr.io/loft-sh/devpod-pro:latest")
	}

	cmd.Log.Infof("Start DevPod Pro via 'docker %s'", strings.Join(args, " "))
	runCmd := cmd.buildDockerCmd(ctx, args...)
	runCmd.Stdout = os.Stdout
	runCmd.Stderr = os.Stderr
	err := runCmd.Run()
	if err != nil {
		return "", err
	}

	return cmd.findLoftContainer(ctx, name, false)
}

func (cmd *StartCmd) logsContainer(ctx context.Context, id string) (string, error) {
	args := []string{"logs", id}
	out, err := cmd.buildDockerCmd(ctx, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("logs container: %w", WrapCommandError(out, err))
	}

	return string(out), nil
}

func (cmd *StartCmd) inspectContainer(ctx context.Context, id string) (*ContainerDetails, error) {
	args := []string{"inspect", "--type", "container", id}
	out, err := cmd.buildDockerCmd(ctx, args...).Output()
	if err != nil {
		return nil, fmt.Errorf("inspect container: %w", WrapCommandError(out, err))
	}

	containerDetails := []*ContainerDetails{}
	err = json.Unmarshal(out, &containerDetails)
	if err != nil {
		return nil, fmt.Errorf("parse inspect output: %w", err)
	} else if len(containerDetails) == 0 {
		return nil, fmt.Errorf("coudln't find container %s", id)
	}

	return containerDetails[0], nil
}

func (cmd *StartCmd) removeContainer(ctx context.Context, id string) error {
	args := []string{"rm", id}
	out, err := cmd.buildDockerCmd(ctx, args...).Output()
	if err != nil {
		return fmt.Errorf("remove container: %w", WrapCommandError(out, err))
	}

	return nil
}

func (cmd *StartCmd) findLoftContainer(ctx context.Context, name string, onlyRunning bool) (string, error) {
	args := []string{"ps", "-q", "-a", "-f", "name=^" + name + "$"}
	out, err := cmd.buildDockerCmd(ctx, args...).Output()
	if err != nil {
		// fallback to manual search
		return "", fmt.Errorf("error finding container: %w", WrapCommandError(out, err))
	}

	arr := []string{}
	scan := scanner.NewScanner(bytes.NewReader(out))
	for scan.Scan() {
		arr = append(arr, strings.TrimSpace(scan.Text()))
	}
	if len(arr) == 0 {
		return "", nil
	}

	// remove the failed / exited containers
	runningContainerID := ""
	for _, containerID := range arr {
		containerState, err := cmd.inspectContainer(ctx, containerID)
		if err != nil {
			return "", err
		} else if onlyRunning && strings.ToLower(containerState.State.Status) != "running" {
			err = cmd.removeContainer(ctx, containerID)
			if err != nil {
				return "", err
			}
		} else {
			runningContainerID = containerID
		}
	}

	return runningContainerID, nil
}

func (cmd *StartCmd) buildDockerCmd(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "docker", args...)
}
