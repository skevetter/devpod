package compose

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/blang/semver/v4"
	composecli "github.com/compose-spec/compose-go/v2/cli"
	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/skevetter/devpod/pkg/docker"
	"github.com/skevetter/log"
)

const (
	ProjectLabel = "com.docker.compose.project"
	ServiceLabel = "com.docker.compose.service"
)

func LoadDockerComposeProject(ctx context.Context, paths []string, envFiles []string) (*composetypes.Project, error) {
	projectOptions, err := composecli.NewProjectOptions(
		paths,
		composecli.WithOsEnv,
		composecli.WithEnvFiles(envFiles...),
		composecli.WithDotEnv,
		composecli.WithDefaultProfiles(),
	)
	if err != nil {
		return nil, err
	}

	project, err := composecli.ProjectFromOptions(ctx, projectOptions)
	if err != nil {
		return nil, err
	}

	return project, nil
}

type ComposeHelper struct {
	Command string
	Version string
	Args    []string
	Docker  *docker.DockerHelper
}

// NewComposeHelper creates a new ComposeHelper instance after detecting whether Docker
// Compose V1 or V2 is installed. It returns an error if neither is found.
func NewComposeHelper(dockerHelper *docker.DockerHelper) (*ComposeHelper, error) {
	dockerCmd := dockerHelper.DockerCommand
	if dockerCmd == "" {
		dockerCmd = "docker"
	}

	if helper, err := tryDockerComposeV2(dockerCmd, dockerHelper.Log); err == nil {
		helper.Docker = dockerHelper
		return helper, nil
	}

	if helper, err := tryDockerComposeV1(); err == nil {
		helper.Docker = dockerHelper
		return helper, nil
	}

	return nil, fmt.Errorf("docker compose not installed")
}

// tryDockerComposeV2 checks if Docker Compose V2 is available and returns a ComposeHelper if so.
// The Compose V2 plugin can be installed on linux using
// sudo curl -SL https://github.com/docker/compose/releases/latest/download/docker-compose-linux-$(uname -m) -o /usr/libexec/docker/cli-plugins/docker-compose
// sudo chmod +x /usr/libexec/docker/cli-plugins/docker-compose
// Docker Compose V2 requires the buildx plugin for building images and can be installed using
// sudo curl -SL https://github.com/docker/buildx/releases/latest/download/buildx-v0.30.1.linux-amd64 -o /usr/libexec/docker/cli-plugins/docker-buildx
// sudo chmod +x /usr/libexec/docker/cli-plugins/docker-buildx.
func tryDockerComposeV2(dockerCmd string, log log.Logger) (*ComposeHelper, error) {
	if exec.Command(dockerCmd, "compose").Run() != nil {
		return nil, fmt.Errorf("docker compose not available")
	}

	out, err := exec.Command(dockerCmd, "compose", "version", "--short").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to get docker compose version %s: %w", string(out), err)
	}

	helper := &ComposeHelper{
		Command: dockerCmd,
		Version: strings.TrimSpace(string(out)),
		Args:    []string{"compose"},
	}

	out, err = exec.Command(dockerCmd, "buildx", "version").CombinedOutput()
	if err != nil {
		// Gracefully handle missing buildx as users might only use compose for running existing images
		log.Errorf("docker buildx not available %s: %w", string(out), err)
	}

	return helper, nil
}

func tryDockerComposeV1() (*ComposeHelper, error) {
	if _, err := exec.LookPath("docker-compose"); err != nil {
		return nil, fmt.Errorf("docker-compose not found in PATH")
	}

	out, err := exec.Command("docker-compose", "version", "--short").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to get docker-compose version %s: %w", string(out), err)
	}

	return &ComposeHelper{
		Command: "docker-compose",
		Version: strings.TrimSpace(string(out)),
		Args:    []string{},
	}, nil
}

func (h *ComposeHelper) FindDevContainer(ctx context.Context, projectName, serviceName string) (*config.ContainerDetails, error) {
	containerIDs, err := h.Docker.FindContainer(ctx, []string{
		fmt.Sprintf("%s=%s", ProjectLabel, projectName),
		fmt.Sprintf("%s=%s", ServiceLabel, serviceName),
	})
	if err != nil {
		return nil, err
	} else if len(containerIDs) == 0 {
		return nil, nil
	}

	containerDetails, err := h.Docker.InspectContainers(ctx, containerIDs)
	if err != nil {
		return nil, err
	}

	for _, details := range containerDetails {
		if details.State.Status != "removing" {
			return &details, nil
		}
	}

	return nil, nil
}

func (h *ComposeHelper) Run(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	cmd := h.buildCmd(ctx, args...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout

	var stderrBuf strings.Builder
	if stderr != nil {
		cmd.Stderr = io.MultiWriter(stderr, &stderrBuf)
	} else {
		cmd.Stderr = &stderrBuf
	}

	err := cmd.Run()
	if err != nil && stderrBuf.Len() > 0 {
		return fmt.Errorf("%s: %w", strings.TrimSpace(stderrBuf.String()), err)
	}
	return err
}

func (h *ComposeHelper) Stop(ctx context.Context, projectName string, args []string) error {
	buildArgs := []string{"--project-name", projectName}
	buildArgs = append(buildArgs, args...)
	buildArgs = append(buildArgs, "stop")

	out, err := h.buildCmd(ctx, buildArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", string(out), err)
	}

	return nil
}

func (h *ComposeHelper) Remove(ctx context.Context, projectName string, args []string) error {
	buildArgs := []string{"--project-name", projectName}
	buildArgs = append(buildArgs, args...)
	buildArgs = append(buildArgs, "down")

	out, err := h.buildCmd(ctx, buildArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", string(out), err)
	}

	return nil
}

func (h *ComposeHelper) GetDefaultImage(projectName, serviceName string) (string, error) {
	version, err := parseVersion(h.Version)
	if err != nil {
		return "", err
	}

	earlierVersion, err := semver.New("2.8.0")
	if err != nil {
		return "", err
	}

	if version.Compare(*earlierVersion) == -1 {
		return fmt.Sprintf("%s_%s", projectName, serviceName), nil
	}

	return fmt.Sprintf("%s-%s", projectName, serviceName), nil
}

func (h *ComposeHelper) FindProjectFiles(ctx context.Context, projectName string) ([]string, error) {
	buildArgs := []string{"--project-name", projectName}
	buildArgs = append(buildArgs, "ls", "-a", "--filter", "name="+projectName, "--format", "json")

	rawOut, err := h.buildCmd(ctx, buildArgs...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", string(rawOut), err)
	}

	type composeOutput struct {
		Name        string
		Status      string
		ConfigFiles string
	}
	var composeOutputs []composeOutput
	if err := json.Unmarshal(rawOut, &composeOutputs); err != nil {
		return nil, fmt.Errorf("parse compose output: %w", err)
	}

	// no compose project found
	if len(composeOutputs) == 0 {
		return nil, nil
	}

	// Parse project files of first match
	projectFiles := strings.Split(composeOutputs[0].ConfigFiles, ",")
	return projectFiles, nil
}

func (h *ComposeHelper) GetProjectName(runnerID string) string {
	// Check for project name override - https://docs.docker.com/compose/how-tos/project-name/
	if projectNameOverride := os.Getenv("COMPOSE_PROJECT_NAME"); projectNameOverride != "" {
		return projectNameOverride
	}
	return h.toProjectName(runnerID)
}

func (h *ComposeHelper) toProjectName(projectName string) string {
	useNewProjectNameFormat, _ := h.useNewProjectName()
	if !useNewProjectNameFormat {
		return regexp.MustCompile("[^a-z0-9]").ReplaceAllString(strings.ToLower(projectName), "")
	}

	return regexp.MustCompile("[^-_a-z0-9]").ReplaceAllString(strings.ToLower(projectName), "")
}

func (h *ComposeHelper) buildCmd(ctx context.Context, args ...string) *exec.Cmd {
	var allArgs []string
	allArgs = append(allArgs, h.Args...)
	allArgs = append(allArgs, args...)
	return exec.CommandContext(ctx, h.Command, allArgs...)
}

// parseVersion extracts and parses the semver portion from version strings.
// Handles non-standard formats like Ubuntu packages (2.37.1+ds1-0ubuntu2~24.04.1)
// and desktop versions (2.40.3-desktop.1) by extracting only major.minor.patch.
func parseVersion(version string) (semver.Version, error) {
	version = strings.TrimPrefix(version, "v")
	re := regexp.MustCompile(`^(\d+\.\d+\.\d+)`)
	matches := re.FindStringSubmatch(version)
	if len(matches) < 2 {
		return semver.Version{}, fmt.Errorf("invalid version format: %s", version)
	}
	return semver.Parse(matches[1])
}

func (h *ComposeHelper) useNewProjectName() (bool, error) {
	version, err := parseVersion(h.Version)
	if err != nil {
		return false, err
	}

	earlierVersion, err := semver.New("1.12.0")
	if err != nil {
		return false, err
	}

	if version.Compare(*earlierVersion) == -1 {
		return false, nil
	}

	return true, nil
}
