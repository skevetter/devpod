package config

import (
	"maps"
	"slices"
)

const UserLabel = "devpod.user"

type Result struct {
	DevContainerConfigWithPath *DevContainerConfigWithPath `json:"DevContainerConfigWithPath"`
	MergedConfig               *MergedDevContainerConfig   `json:"MergedConfig"`
	SubstitutionContext        *SubstitutionContext        `json:"SubstitutionContext"`
	ContainerDetails           *ContainerDetails           `json:"ContainerDetails"`
}

type DevContainerConfigWithPath struct {
	// Config is the devcontainer.json config
	Config *DevContainerConfig `json:"config,omitempty"`

	// Path is the relative path to the devcontainer.json from the workspace folder
	Path string `json:"path,omitempty"`
}

func GetMounts(result *Result) []*Mount {
	workspaceMount := ParseMount(result.SubstitutionContext.WorkspaceMount)
	mounts := []*Mount{&workspaceMount}
	for _, m := range result.MergedConfig.Mounts {
		if m.Type == "bind" {
			mounts = append(mounts, m)
		}
	}

	return mounts
}

// GetRemoteUser determines the remote user using DevContainer specification priority order:
// 1. remoteUser from configuration
// 2. devpod.user label from container
// 3. User field from Docker inspect
// 4. containerUser from configuration
//
// Per DevContainer specification (https://containers.dev/implementors/json_reference/):
// "remoteUser: Overrides the user that devcontainer.json supporting services tools / runs as in the container...
// Defaults to the user the container as a whole is running as (often root)."
func GetRemoteUser(result *Result) string {
	if result == nil {
		return "root"
	}

	if result.MergedConfig != nil && result.MergedConfig.RemoteUser != "" {
		return result.MergedConfig.RemoteUser
	}

	if result.ContainerDetails != nil && result.ContainerDetails.Config.Labels != nil {
		if userLabel := result.ContainerDetails.Config.Labels[UserLabel]; userLabel != "" {
			return userLabel
		}
	}

	if result.MergedConfig != nil && result.MergedConfig.ContainerUser != "" {
		return result.MergedConfig.ContainerUser
	}

	return "root"
}

func GetDevPodCustomizations(parsedConfig *DevContainerConfig) *DevPodCustomizations {
	if parsedConfig.Customizations == nil || parsedConfig.Customizations["devpod"] == nil {
		return &DevPodCustomizations{}
	}

	devPod := &DevPodCustomizations{}
	err := Convert(parsedConfig.Customizations["devpod"], devPod)
	if err != nil {
		return &DevPodCustomizations{}
	}

	return devPod
}

func GetVSCodeConfiguration(mergedConfig *MergedDevContainerConfig) *VSCodeCustomizations {
	if mergedConfig.Customizations == nil || mergedConfig.Customizations["vscode"] == nil {
		return &VSCodeCustomizations{}
	}

	retVSCodeCustomizations := &VSCodeCustomizations{
		Settings:   map[string]any{},
		Extensions: nil,
	}
	for _, customization := range mergedConfig.Customizations["vscode"] {
		vsCode := &VSCodeCustomizations{}
		err := Convert(customization, vsCode)
		if err != nil {
			continue
		}

		for _, extension := range vsCode.Extensions {
			if contains(retVSCodeCustomizations.Extensions, extension) {
				continue
			}

			retVSCodeCustomizations.Extensions = append(retVSCodeCustomizations.Extensions, extension)
		}

		maps.Copy(retVSCodeCustomizations.Settings, vsCode.Settings)
	}

	return retVSCodeCustomizations
}

func GetJetBrainsConfiguration(mergedConfig *MergedDevContainerConfig) *JetBrainsCustomizations {
	if mergedConfig.Customizations == nil || mergedConfig.Customizations["jetbrains"] == nil {
		return &JetBrainsCustomizations{}
	}

	retJetBrainsCustomizations := &JetBrainsCustomizations{
		Plugins: nil,
	}
	for _, customization := range mergedConfig.Customizations["jetbrains"] {
		jetBrains := &JetBrainsCustomizations{}
		err := Convert(customization, jetBrains)
		if err != nil {
			continue
		}

		// We ignore "backend" because IDE choice is made locally.
		// We ignore "settings" because there is no way to apply them (at least not via `remote-dev-server.sh`).

		for _, plugin := range jetBrains.Plugins {
			if contains(retJetBrainsCustomizations.Plugins, plugin) {
				continue
			}

			retJetBrainsCustomizations.Plugins = append(retJetBrainsCustomizations.Plugins, plugin)
		}
	}

	return retJetBrainsCustomizations
}

func contains(stack []string, k string) bool {
	return slices.Contains(stack, k)
}
