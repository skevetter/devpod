package options

import (
	"context"
	"fmt"
	"maps"
	"os"
	"reflect"
	"strings"

	"github.com/skevetter/devpod/pkg/agent"
	"github.com/skevetter/devpod/pkg/config"
	"github.com/skevetter/devpod/pkg/options/resolver"
	"github.com/skevetter/devpod/pkg/provider"
	"github.com/skevetter/devpod/pkg/types"
	"github.com/skevetter/log"
)

func ResolveAndSaveOptionsMachine(
	ctx context.Context,
	devConfig *config.Config,
	providerConfig *provider.ProviderConfig,
	originalMachine *provider.Machine,
	userOptions map[string]string,
	log log.Logger,
) (*provider.Machine, error) {
	if originalMachine == nil {
		return nil, fmt.Errorf("originalMachine cannot be nil")
	}
	// reload config
	machine, err := provider.LoadMachineConfig(originalMachine.Context, originalMachine.ID)
	if err != nil {
		return originalMachine, err
	}

	// resolve devconfig options
	var beforeConfigOptions map[string]config.OptionValue
	if machine != nil {
		beforeConfigOptions = machine.Provider.Options
	}

	// get binary paths
	binaryPaths, err := provider.GetBinaries(devConfig.DefaultContext, providerConfig)
	if err != nil {
		return nil, err
	}

	// resolve options
	resolvedOptions, _, err := resolver.New(
		userOptions,
		provider.Merge(provider.ToOptionsMachine(machine), binaryPaths),
		log,
		resolver.WithResolveLocal(),
	).Resolve(
		ctx,
		devConfig.DynamicProviderOptionDefinitions(providerConfig.Name),
		providerConfig.Options,
		provider.CombineOptions(nil, machine, devConfig.ProviderOptions(providerConfig.Name)),
	)
	if err != nil {
		return nil, err
	}

	// remove global options
	filterResolvedOptions(
		resolvedOptions,
		beforeConfigOptions,
		devConfig.ProviderOptions(providerConfig.Name),
		providerConfig.Options,
		userOptions,
	)

	// save machine config
	if machine != nil {
		machine.Provider.Options = resolvedOptions

		if !reflect.DeepEqual(beforeConfigOptions, machine.Provider.Options) {
			err = provider.SaveMachineConfig(machine)
			if err != nil {
				return machine, err
			}
		}
	}

	return machine, nil
}

func ResolveAndSaveOptionsWorkspace(
	ctx context.Context,
	devConfig *config.Config,
	providerConfig *provider.ProviderConfig,
	originalWorkspace *provider.Workspace,
	userOptions map[string]string,
	log log.Logger,
	options ...resolver.Option,
) (*provider.Workspace, error) {
	if originalWorkspace == nil {
		return nil, fmt.Errorf("originalWorkspace cannot be nil")
	}
	// reload config
	workspace, err := provider.LoadWorkspaceConfig(originalWorkspace.Context, originalWorkspace.ID)
	if err != nil {
		return originalWorkspace, err
	}
	if workspace == nil {
		return nil, fmt.Errorf("failed to load workspace config: workspace not found")
	}

	// resolve devconfig options
	beforeConfigOptions := workspace.Provider.Options

	// get binary paths
	binaryPaths, err := provider.GetBinaries(devConfig.DefaultContext, providerConfig)
	if err != nil {
		return nil, err
	}
	options = append(options, resolver.WithResolveLocal())

	// resolve options
	resolvedOptions, _, err := resolver.New(
		userOptions,
		provider.Merge(provider.ToOptionsWorkspace(workspace), binaryPaths),
		log,
		options...,
	).Resolve(
		ctx,
		devConfig.DynamicProviderOptionDefinitions(providerConfig.Name),
		providerConfig.Options,
		provider.CombineOptions(workspace, nil, devConfig.ProviderOptions(providerConfig.Name)),
	)
	if err != nil {
		return nil, err
	}

	// remove global options
	filterResolvedOptions(
		resolvedOptions,
		beforeConfigOptions,
		devConfig.ProviderOptions(providerConfig.Name),
		providerConfig.Options,
		userOptions,
	)

	// save workspace config
	workspace.Provider.Options = resolvedOptions
	if !reflect.DeepEqual(beforeConfigOptions, workspace.Provider.Options) {
		err = provider.SaveWorkspaceConfig(workspace)
		if err != nil {
			return workspace, err
		}
	}

	return workspace, nil
}

func ResolveOptions(
	ctx context.Context,
	devConfig *config.Config,
	providerConfig *provider.ProviderConfig,
	userOptions map[string]string,
	skipRequired bool,
	skipSubOptions bool,
	singleMachine *bool,
	log log.Logger,
) (*config.Config, error) {
	// get binary paths
	binaryPaths, err := provider.GetBinaries(devConfig.DefaultContext, providerConfig)
	if err != nil {
		return nil, err
	}

	resolverOpts := []resolver.Option{
		resolver.WithResolveGlobal(),
		resolver.WithSkipRequired(skipRequired),
	}
	if !skipSubOptions {
		resolverOpts = append(resolverOpts, resolver.WithResolveSubOptions())
	}

	// create new resolver
	resolve := resolver.New(
		userOptions,
		provider.Merge(provider.GetBaseEnvironment(devConfig.DefaultContext, providerConfig.Name), binaryPaths),
		log,
		resolverOpts...,
	)

	// loop and resolve options, as soon as we encounter a new dynamic option it will get filled
	resolvedOptionValues, dynamicOptionDefinitions, err := resolve.Resolve(
		ctx,
		nil,
		providerConfig.Options,
		devConfig.ProviderOptions(providerConfig.Name),
	)
	if err != nil {
		return nil, err
	}

	// save options in dev config
	if devConfig != nil {
		devConfig = config.CloneConfig(devConfig)
		if devConfig.Current().Providers == nil {
			devConfig.Current().Providers = map[string]*config.ProviderConfig{}
		}
		if devConfig.Current().Providers[providerConfig.Name] == nil {
			devConfig.Current().Providers[providerConfig.Name] = &config.ProviderConfig{}
		}

		providerCfg := devConfig.Current().Providers[providerConfig.Name]
		providerCfg.Options = map[string]config.OptionValue{}
		maps.Copy(providerCfg.Options, resolvedOptionValues)

		providerCfg.DynamicOptions = config.OptionDefinitions{}
		maps.Copy(providerCfg.DynamicOptions, dynamicOptionDefinitions)
		if singleMachine != nil {
			providerCfg.SingleMachine = *singleMachine
		}
	}

	return devConfig, nil
}

// ResolveAgentConfig resolves and returns the complete agent configuration for a provider.
// It merges configuration from the provider, workspace, machine, and devConfig, resolving
// all dynamic values and setting appropriate defaults for agent paths, Docker settings,
// Kubernetes settings, and credentials.
//
// Parameters:
//   - devConfig: The DevPod configuration containing global settings
//   - providerConfig: The provider's configuration
//   - workspace: The workspace configuration (can be nil for machine-only operations)
//   - machine: The machine configuration (can be nil for workspace-only operations)
//
// Returns a fully resolved ProviderAgentConfig ready for use by the agent.
func ResolveAgentConfig(
	devConfig *config.Config,
	providerConfig *provider.ProviderConfig,
	workspace *provider.Workspace,
	machine *provider.Machine,
) provider.ProviderAgentConfig {
	if providerConfig == nil || devConfig == nil {
		return provider.ProviderAgentConfig{}
	}
	options := provider.ToOptions(workspace, machine, devConfig.ProviderOptions(providerConfig.Name))
	agentConfig := providerConfig.Agent

	resolveAgentBaseConfig(&agentConfig, options, devConfig)
	resolveAgentDockerConfig(&agentConfig, options)
	resolveAgentKubernetesConfig(&agentConfig, options)
	resolveAgentPathAndURL(&agentConfig, options, devConfig)
	resolveAgentCredentials(&agentConfig, options, devConfig)

	return agentConfig
}

func resolveAgentBaseConfig(
	agentConfig *provider.ProviderAgentConfig,
	options map[string]string,
	devConfig *config.Config,
) {
	agentConfig.Dockerless.Image = resolver.ResolveDefaultValue(agentConfig.Dockerless.Image, options)
	agentConfig.Dockerless.Disabled = types.StrBool(
		resolver.ResolveDefaultValue(string(agentConfig.Dockerless.Disabled), options),
	)
	agentConfig.Dockerless.IgnorePaths = resolver.ResolveDefaultValue(agentConfig.Dockerless.IgnorePaths, options)
	agentConfig.Dockerless.RegistryCache = devConfig.ContextOption(config.ContextOptionRegistryCache)
	agentConfig.Driver = resolver.ResolveDefaultValue(agentConfig.Driver, options)
	agentConfig.Local = types.StrBool(resolver.ResolveDefaultValue(string(agentConfig.Local), options))
}

func resolveAgentDockerConfig(agentConfig *provider.ProviderAgentConfig, options map[string]string) {
	agentConfig.Docker.Path = resolver.ResolveDefaultValue(agentConfig.Docker.Path, options)
	agentConfig.Docker.Builder = resolver.ResolveDefaultValue(agentConfig.Docker.Builder, options)
	agentConfig.Docker.Install = types.StrBool(
		resolver.ResolveDefaultValue(string(agentConfig.Docker.Install), options),
	)
	agentConfig.Docker.Env = resolver.ResolveDefaultValues(agentConfig.Docker.Env, options)
}

func resolveAgentKubernetesConfig(agentConfig *provider.ProviderAgentConfig, options map[string]string) {
	k8s := &agentConfig.Kubernetes
	k8s.KubernetesContext = resolver.ResolveDefaultValue(k8s.KubernetesContext, options)
	k8s.KubernetesConfig = resolver.ResolveDefaultValue(k8s.KubernetesConfig, options)
	k8s.KubernetesNamespace = resolver.ResolveDefaultValue(k8s.KubernetesNamespace, options)
	k8s.Architecture = resolver.ResolveDefaultValue(k8s.Architecture, options)
	k8s.InactivityTimeout = resolver.ResolveDefaultValue(k8s.InactivityTimeout, options)
	k8s.StorageClass = resolver.ResolveDefaultValue(k8s.StorageClass, options)
	k8s.PvcAccessMode = resolver.ResolveDefaultValue(k8s.PvcAccessMode, options)
	k8s.PvcAnnotations = resolver.ResolveDefaultValue(k8s.PvcAnnotations, options)
	k8s.NodeSelector = resolver.ResolveDefaultValue(k8s.NodeSelector, options)
	k8s.Resources = resolver.ResolveDefaultValue(k8s.Resources, options)
	k8s.WorkspaceVolumeMount = resolver.ResolveDefaultValue(k8s.WorkspaceVolumeMount, options)
	k8s.PodManifestTemplate = resolver.ResolveDefaultValue(k8s.PodManifestTemplate, options)
	k8s.Labels = resolver.ResolveDefaultValue(k8s.Labels, options)
	k8s.StrictSecurity = resolver.ResolveDefaultValue(k8s.StrictSecurity, options)
	k8s.CreateNamespace = resolver.ResolveDefaultValue(k8s.CreateNamespace, options)
	k8s.ClusterRole = resolver.ResolveDefaultValue(k8s.ClusterRole, options)
	k8s.ServiceAccount = resolver.ResolveDefaultValue(k8s.ServiceAccount, options)
	k8s.PodTimeout = resolver.ResolveDefaultValue(k8s.PodTimeout, options)
	k8s.KubernetesPullSecretsEnabled = resolver.ResolveDefaultValue(k8s.KubernetesPullSecretsEnabled, options)
	k8s.DiskSize = resolver.ResolveDefaultValue(k8s.DiskSize, options)
}

func resolveAgentPathAndURL(
	agentConfig *provider.ProviderAgentConfig,
	options map[string]string,
	devConfig *config.Config,
) {
	agentConfig.DataPath = resolver.ResolveDefaultValue(agentConfig.DataPath, options)
	agentConfig.Path = resolver.ResolveDefaultValue(agentConfig.Path, options)
	if agentConfig.Path == "" && strings.EqualFold(string(agentConfig.Local), "true") {
		// Try to use the current executable path for local agent
		// Error is silently handled as we have a fallback to RemoteDevPodHelperLocation
		if execPath, err := os.Executable(); err == nil {
			agentConfig.Path = execPath
		}
	}
	if agentConfig.Path == "" {
		agentConfig.Path = agent.RemoteDevPodHelperLocation
	}
	agentConfig.DownloadURL = resolver.ResolveDefaultValue(agentConfig.DownloadURL, options)
	if agentConfig.DownloadURL == "" {
		agentConfig.DownloadURL = resolveAgentDownloadURL(devConfig)
	}
	agentConfig.Timeout = resolver.ResolveDefaultValue(agentConfig.Timeout, options)
	agentConfig.ContainerTimeout = resolver.ResolveDefaultValue(agentConfig.ContainerTimeout, options)
}

func resolveAgentCredentials(
	agentConfig *provider.ProviderAgentConfig,
	options map[string]string,
	devConfig *config.Config,
) {
	agentConfig.InjectGitCredentials = types.StrBool(
		resolver.ResolveDefaultValue(string(agentConfig.InjectGitCredentials), options),
	)
	if devConfig.ContextOption(config.ContextOptionSSHInjectGitCredentials) != "" {
		agentConfig.InjectGitCredentials = types.StrBool(
			devConfig.ContextOption(config.ContextOptionSSHInjectGitCredentials),
		)
	}
	agentConfig.InjectDockerCredentials = types.StrBool(
		resolver.ResolveDefaultValue(string(agentConfig.InjectDockerCredentials), options),
	)
	if dockerCredOpt := devConfig.ContextOption(config.ContextOptionSSHInjectDockerCredentials); dockerCredOpt != "" {
		agentConfig.InjectDockerCredentials = types.StrBool(dockerCredOpt)
	}
}

// resolveAgentDownloadURL resolves the agent download URL (env -> context -> default).
func resolveAgentDownloadURL(devConfig *config.Config) string {
	devPodAgentURL := os.Getenv(agent.EnvDevPodAgentURL)
	if devPodAgentURL != "" {
		return strings.TrimSuffix(devPodAgentURL, "/") + "/"
	}

	contextAgentOption, ok := devConfig.Current().Options[config.ContextOptionAgentURL]
	if ok && contextAgentOption.Value != "" {
		return strings.TrimSuffix(contextAgentOption.Value, "/") + "/"
	}

	return agent.DefaultAgentDownloadURL()
}

func filterResolvedOptions(resolvedOptions, beforeConfigOptions, providerValues map[string]config.OptionValue, providerOptions map[string]*types.Option, userOptions map[string]string) {
	for k := range resolvedOptions {
		// check if user supplied
		if userOptions != nil {
			_, ok := userOptions[k]
			if ok {
				continue
			}
		}

		// check if it was there before
		if beforeConfigOptions != nil {
			_, ok := beforeConfigOptions[k]
			if ok {
				continue
			}
		}

		// check if not available in the provider values
		if providerValues != nil {
			_, ok := providerValues[k]
			if !ok {
				continue
			}
		}

		// check if not global
		if providerOptions == nil || providerOptions[k] == nil || !providerOptions[k].Global {
			continue
		}

		delete(resolvedOptions, k)
	}
}
