package clientimplementation

import (
	"encoding/json"

	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/pkg/compress"
	"github.com/skevetter/devpod/pkg/config"
	config2 "github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/skevetter/devpod/pkg/options"
	"github.com/skevetter/devpod/pkg/provider"
)

func (s *workspaceClient) AgentLocal() bool {
	s.m.Lock()
	defer s.m.Unlock()

	return options.ResolveAgentConfig(s.devPodConfig, s.config, s.workspace, s.machine).Local == "true"
}

func (s *workspaceClient) AgentPath() string {
	s.m.Lock()
	defer s.m.Unlock()

	return options.ResolveAgentConfig(s.devPodConfig, s.config, s.workspace, s.machine).Path
}

func (s *workspaceClient) AgentURL() string {
	s.m.Lock()
	defer s.m.Unlock()

	return options.ResolveAgentConfig(s.devPodConfig, s.config, s.workspace, s.machine).DownloadURL
}

func (s *workspaceClient) AgentInjectGitCredentials(cliOptions provider.CLIOptions) bool {
	s.m.Lock()
	defer s.m.Unlock()

	return s.agentInfo(cliOptions).Agent.InjectGitCredentials == "true"
}

func (s *workspaceClient) AgentInjectDockerCredentials(cliOptions provider.CLIOptions) bool {
	s.m.Lock()
	defer s.m.Unlock()

	return s.agentInfo(cliOptions).Agent.InjectDockerCredentials == "true"
}

func (s *workspaceClient) AgentInfo(cliOptions provider.CLIOptions) (string, *provider.AgentWorkspaceInfo, error) {
	s.m.Lock()
	defer s.m.Unlock()

	return s.compressedAgentInfo(cliOptions)
}

func (s *workspaceClient) compressedAgentInfo(cliOptions provider.CLIOptions) (string, *provider.AgentWorkspaceInfo, error) {
	agentInfo := s.agentInfo(cliOptions)

	// marshal config
	out, err := json.Marshal(agentInfo)
	if err != nil {
		return "", nil, err
	}

	compressed, err := compress.Compress(string(out))
	if err != nil {
		return "", nil, err
	}

	return compressed, agentInfo, nil
}

func (s *workspaceClient) agentInfo(cliOptions provider.CLIOptions) *provider.AgentWorkspaceInfo {
	// try to load last devcontainer.json
	var lastDevContainerConfig *config2.DevContainerConfigWithPath
	var workspaceOrigin string
	if s.workspace != nil {
		result, err := provider.LoadWorkspaceResult(s.workspace.Context, s.workspace.ID)
		if err != nil {
			s.log.WithFields(logrus.Fields{"error": err}).Debug("error loading workspace result")
		} else if result != nil {
			lastDevContainerConfig = result.DevContainerConfigWithPath
		}

		workspaceOrigin = s.workspace.Origin
	}

	// build struct
	agentInfo := &provider.AgentWorkspaceInfo{
		WorkspaceOrigin:        workspaceOrigin,
		Workspace:              s.workspace,
		Machine:                s.machine,
		LastDevContainerConfig: lastDevContainerConfig,
		CLIOptions:             cliOptions,
		Agent:                  options.ResolveAgentConfig(s.devPodConfig, s.config, s.workspace, s.machine),
		Options:                s.devPodConfig.ProviderOptions(s.Provider()),
	}

	// if we are running platform mode
	if cliOptions.Platform.Enabled {
		agentInfo.Agent.InjectGitCredentials = "true"
		agentInfo.Agent.InjectDockerCredentials = "true"
	}

	// we don't send any provider options if proxy because these could contain
	// sensitive information and we don't want to allow privileged containers that
	// have access to the host to save these.
	if agentInfo.Agent.Driver != provider.CustomDriver && (cliOptions.Platform.Enabled || cliOptions.DisableDaemon) {
		agentInfo.Options = map[string]config.OptionValue{}
		agentInfo.Workspace = provider.CloneWorkspace(agentInfo.Workspace)
		agentInfo.Workspace.Provider.Options = map[string]config.OptionValue{}
		if agentInfo.Machine != nil {
			agentInfo.Machine = provider.CloneMachine(agentInfo.Machine)
			agentInfo.Machine.Provider.Options = map[string]config.OptionValue{}
		}
	}

	// Get the timeout from the context options
	agentInfo.InjectTimeout = config.ParseTimeOption(s.devPodConfig, config.ContextOptionAgentInjectTimeout)

	// Set registry cache from context option
	agentInfo.RegistryCache = s.devPodConfig.ContextOption(config.ContextOptionRegistryCache)

	return agentInfo
}
