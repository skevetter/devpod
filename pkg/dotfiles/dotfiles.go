package dotfiles

import (
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/pkg/agent"
	client2 "github.com/skevetter/devpod/pkg/client"
	"github.com/skevetter/devpod/pkg/config"
	config2 "github.com/skevetter/devpod/pkg/devcontainer/config"
	devssh "github.com/skevetter/devpod/pkg/ssh"
	"github.com/skevetter/log"
)

// SetupParams holds all parameters needed for dotfiles setup.
type SetupParams struct {
	Source       string
	Script       string
	EnvFiles     []string
	EnvKeyValues []string
	Client       client2.BaseWorkspaceClient
	DevPodConfig *config.Config
	Log          log.Logger
}

// Setup clones and installs dotfiles into the devcontainer.
func Setup(p SetupParams) error {
	dotfilesRepo := p.DevPodConfig.ContextOption(config.ContextOptionDotfilesURL)
	if p.Source != "" {
		dotfilesRepo = p.Source
	}

	dotfilesScript := p.DevPodConfig.ContextOption(config.ContextOptionDotfilesScript)
	if p.Script != "" {
		dotfilesScript = p.Script
	}

	if dotfilesRepo == "" {
		p.Log.Debug("No dotfiles repo specified, skipping")
		return nil
	}

	p.Log.Infof("Dotfiles Git repository %s specified", dotfilesRepo)
	p.Log.Debug("Cloning dotfiles into the devcontainer...")

	dotCmd, err := buildDotCmd(buildDotCmdParams{
		devPodConfig:     p.DevPodConfig,
		dotfilesRepo:     dotfilesRepo,
		dotfilesScript:   dotfilesScript,
		envFiles:         p.EnvFiles,
		envKeyValuePairs: p.EnvKeyValues,
		client:           p.Client,
		log:              p.Log,
	})
	if err != nil {
		return err
	}
	if p.Log.GetLevel() == logrus.DebugLevel {
		dotCmd.Args = append(dotCmd.Args, "--debug")
	}

	p.Log.Debugf("Running dotfiles setup command: %v", dotCmd.Args)

	writer := p.Log.Writer(logrus.InfoLevel, false)

	dotCmd.Stdout = writer
	dotCmd.Stderr = writer

	err = dotCmd.Run()
	if err != nil {
		return err
	}

	p.Log.Infof("Done setting up dotfiles into the devcontainer")

	return nil
}

func buildDotCmdAgentArguments(
	dotfilesRepo, dotfilesScript string,
	strictHostKey, debug bool,
) []string {
	agentArguments := []string{
		"agent",
		"workspace",
		"install-dotfiles",
		"--repository",
		dotfilesRepo,
	}

	if strictHostKey {
		agentArguments = append(agentArguments, "--strict-host-key-checking")
	}

	if debug {
		agentArguments = append(agentArguments, "--debug")
	}

	if dotfilesScript != "" {
		agentArguments = append(agentArguments, "--install-script", dotfilesScript)
	}

	return agentArguments
}

type buildDotCmdParams struct {
	devPodConfig     *config.Config
	dotfilesRepo     string
	dotfilesScript   string
	envFiles         []string
	envKeyValuePairs []string
	client           client2.BaseWorkspaceClient
	log              log.Logger
}

func buildDotCmd(p buildDotCmdParams) (*exec.Cmd, error) {
	sshCmd := []string{
		"ssh",
		"--agent-forwarding=true",
		"--start-services=true",
	}

	envFilesKeyValuePairs, err := collectDotfilesScriptEnvKeyValuePairs(p.envFiles)
	if err != nil {
		return nil, err
	}

	// Collect file-based and CLI options env variables names (aka keys) and
	// configure ssh env var passthrough with send-env
	allEnvKeyValuesPairs := slices.Concat(envFilesKeyValuePairs, p.envKeyValuePairs)
	allEnvKeys := extractKeysFromEnvKeyValuePairs(allEnvKeyValuesPairs)
	for _, envKey := range allEnvKeys {
		sshCmd = append(sshCmd, "--send-env", envKey)
	}

	remoteUser, err := devssh.GetUser(
		p.client.WorkspaceConfig().ID,
		p.client.WorkspaceConfig().SSHConfigPath,
		p.client.WorkspaceConfig().SSHConfigIncludePath,
	)
	if err != nil {
		remoteUser = "root"
	}

	strictHostKey := p.devPodConfig.ContextOption(
		config.ContextOptionSSHStrictHostKeyChecking,
	) == config.BoolTrue
	debug := p.log.GetLevel() == logrus.DebugLevel
	agentArguments := buildDotCmdAgentArguments(
		p.dotfilesRepo, p.dotfilesScript, strictHostKey, debug,
	)

	if p.dotfilesScript != "" {
		p.log.Infof("Dotfiles script %s specified", p.dotfilesScript)
	}

	sshCmd = append(sshCmd,
		"--user",
		remoteUser,
		"--context",
		p.client.Context(),
		p.client.Workspace(),
		"--log-output=raw",
		"--command",
		agent.ContainerDevPodHelperLocation+" "+strings.Join(agentArguments, " "),
	)
	execPath, err := os.Executable()
	if err != nil {
		return nil, err
	}

	dotCmd := exec.Command( //nolint:gosec
		execPath,
		sshCmd...,
	)

	dotCmd.Env = append(dotCmd.Environ(), allEnvKeyValuesPairs...)
	return dotCmd, nil
}

func extractKeysFromEnvKeyValuePairs(envKeyValuePairs []string) []string {
	keys := []string{}
	for _, env := range envKeyValuePairs {
		keyValue := strings.SplitN(env, "=", 2)
		if len(keyValue) == 2 {
			keys = append(keys, keyValue[0])
		}
	}
	return keys
}

func collectDotfilesScriptEnvKeyValuePairs(envFiles []string) ([]string, error) {
	keyValues := []string{}
	for _, file := range envFiles {
		envFromFile, err := config2.ParseKeyValueFile(file)
		if err != nil {
			return nil, err
		}
		keyValues = append(keyValues, envFromFile...)
	}
	return keyValues, nil
}
