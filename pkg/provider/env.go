package provider

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/skevetter/devpod/pkg/config"
	log2 "github.com/skevetter/log"
)

const (
	// workspace.
	WorkspaceID        = "WORKSPACE_ID"
	WorkspaceUID       = "WORKSPACE_UID"
	WorkspacePicture   = "WORKSPACE_PICTURE"
	WorkspaceFolder    = "WORKSPACE_FOLDER"
	WorkspaceContext   = "WORKSPACE_CONTEXT"
	WorkspaceOrigin    = "WORKSPACE_ORIGIN"
	WorkspaceSourceEnv = "WORKSPACE_SOURCE"
	WorkspaceProvider  = "WORKSPACE_PROVIDER"

	// machine.
	MachineID       = "MACHINE_ID"
	MachineContext  = "MACHINE_CONTEXT"
	MachineFolder   = "MACHINE_FOLDER"
	MachineProvider = "MACHINE_PROVIDER"

	// provider.
	ProviderID      = "PROVIDER_ID"
	ProviderContext = "PROVIDER_CONTEXT"
	ProviderFolder  = "PROVIDER_FOLDER"

	// pro.
	LoftProject       = "LOFT_PROJECT"
	LoftFilterByOwner = "LOFT_FILTER_BY_OWNER"
)

const (
	DevcontainerID = "DEVCONTAINER_ID"
)

func combineOptions(
	resolvedOptions map[string]config.OptionValue,
	otherOptions map[string]config.OptionValue,
) map[string]config.OptionValue {
	options := map[string]config.OptionValue{}
	maps.Copy(options, resolvedOptions)
	maps.Copy(options, otherOptions)
	return options
}

func ToEnvironment(
	workspace *Workspace,
	machine *Machine,
	options map[string]config.OptionValue,
	extraEnv map[string]string,
) []string {
	env := ToOptions(workspace, machine, options)

	// create environment variables for command
	osEnviron := os.Environ()
	for k, v := range env {
		osEnviron = append(osEnviron, k+"="+v)
	}
	for k, v := range extraEnv {
		osEnviron = append(osEnviron, k+"="+v)
	}

	return osEnviron
}

func CombineOptions(
	workspace *Workspace,
	machine *Machine,
	options map[string]config.OptionValue,
) map[string]config.OptionValue {
	providerOptions := map[string]config.OptionValue{}
	if options != nil {
		providerOptions = combineOptions(providerOptions, options)
	}
	if workspace != nil {
		providerOptions = combineOptions(providerOptions, workspace.Provider.Options)
	}
	if machine != nil {
		providerOptions = combineOptions(providerOptions, machine.Provider.Options)
	}
	return providerOptions
}

func ToOptionsWorkspace(workspace *Workspace) map[string]string {
	retVars := map[string]string{}
	if workspace != nil {
		if workspace.ID != "" {
			retVars[WorkspaceID] = workspace.ID
		}
		if workspace.UID != "" {
			retVars[WorkspaceUID] = workspace.UID
		}
		retVars[WorkspaceFolder], _ = GetWorkspaceDir(workspace.Context, workspace.ID)
		retVars[WorkspaceFolder] = filepath.ToSlash(retVars[WorkspaceFolder])
		if workspace.Context != "" {
			retVars[WorkspaceContext] = workspace.Context
			retVars[MachineContext] = workspace.Context
		}
		if workspace.Origin != "" {
			retVars[WorkspaceOrigin] = filepath.ToSlash(workspace.Origin)
		}
		if workspace.Picture != "" {
			retVars[WorkspacePicture] = workspace.Picture
		}
		retVars[WorkspaceSourceEnv] = workspace.Source.String()
		if workspace.Provider.Name != "" {
			retVars[WorkspaceProvider] = workspace.Provider.Name
		}
		if workspace.Machine.ID != "" {
			retVars[MachineID] = workspace.Machine.ID
			machineDir, _ := GetMachineDir(workspace.Context, workspace.Machine.ID)
			retVars[MachineFolder] = filepath.ToSlash(machineDir)
		}
		if workspace.Pro != nil && workspace.Pro.Project != "" {
			retVars[LoftProject] = workspace.Pro.Project
		}
		maps.Copy(retVars, GetBaseEnvironment(workspace.Context, workspace.Provider.Name))
	}
	return retVars
}

func ToOptionsMachine(machine *Machine) map[string]string {
	retVars := map[string]string{}
	if machine != nil {
		if machine.ID != "" {
			retVars[MachineID] = machine.ID
		}
		retVars[MachineFolder], _ = GetMachineDir(machine.Context, machine.ID)
		retVars[MachineFolder] = filepath.ToSlash(retVars[MachineFolder])
		if machine.Context != "" {
			retVars[MachineContext] = machine.Context
		}
		if machine.Provider.Name != "" {
			retVars[MachineProvider] = machine.Provider.Name
		}
		maps.Copy(retVars, GetBaseEnvironment(machine.Context, machine.Provider.Name))
	}
	return retVars
}

func ToOptions(
	workspace *Workspace,
	machine *Machine,
	options map[string]config.OptionValue,
) map[string]string {
	providerOptions := CombineOptions(workspace, machine, options)
	retVars := map[string]string{}
	for optionName, optionValue := range providerOptions {
		retVars[strings.ToUpper(optionName)] = optionValue.Value
	}

	retVars = Merge(retVars, ToOptionsWorkspace(workspace))
	retVars = Merge(retVars, ToOptionsMachine(machine))
	return retVars
}

func Merge(m1 map[string]string, m2 map[string]string) map[string]string {
	retMap := map[string]string{}
	maps.Copy(retMap, m1)
	maps.Copy(retMap, m2)

	return retMap
}

func GetBaseEnvironment(context, provider string) map[string]string {
	retVars := map[string]string{}

	// devpod binary
	devPodBinary, _ := os.Executable()
	retVars[config.EnvBinaryPath] = filepath.ToSlash(devPodBinary)
	retVars[config.EnvOS] = runtime.GOOS
	retVars[config.EnvArch] = runtime.GOARCH
	retVars[ProviderID] = provider
	retVars[ProviderContext] = context
	providerFolder, _ := GetProviderDir(context, provider)
	retVars[ProviderFolder] = filepath.ToSlash(providerFolder)
	retVars[config.EnvLogLevel] = log2.Default.GetLevel().String()
	return retVars
}

func GetProviderOptions(
	workspace *Workspace,
	server *Machine,
	devConfig *config.Config,
) map[string]config.OptionValue {
	retValues := map[string]config.OptionValue{}
	providerName := ""
	if workspace != nil {
		providerName = workspace.Provider.Name
	}
	if server != nil {
		providerName = server.Provider.Name
	}
	if devConfig != nil && providerName != "" {
		maps.Copy(retValues, devConfig.Current().ProviderOptions(providerName))
	}
	return retValues
}

func CloneAgentWorkspaceInfo(agentWorkspaceInfo *AgentWorkspaceInfo) *AgentWorkspaceInfo {
	if agentWorkspaceInfo == nil {
		return nil
	}
	out, _ := json.Marshal(agentWorkspaceInfo)
	ret := &AgentWorkspaceInfo{}
	_ = json.Unmarshal(out, ret)
	ret.Origin = agentWorkspaceInfo.Origin
	ret.Workspace = CloneWorkspace(agentWorkspaceInfo.Workspace)
	ret.Machine = CloneMachine(agentWorkspaceInfo.Machine)
	return ret
}

func CloneWorkspace(workspace *Workspace) *Workspace {
	if workspace == nil {
		return nil
	}
	out, _ := json.Marshal(workspace)
	ret := &Workspace{}
	_ = json.Unmarshal(out, ret)
	ret.Origin = workspace.Origin
	return ret
}

func CloneMachine(server *Machine) *Machine {
	if server == nil {
		return nil
	}
	out, _ := json.Marshal(server)
	ret := &Machine{}
	_ = json.Unmarshal(out, ret)
	ret.Origin = server.Origin
	return ret
}
