package ideparse

import (
	"fmt"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/skevetter/devpod/pkg/command"
	"github.com/skevetter/devpod/pkg/config"
	"github.com/skevetter/devpod/pkg/ide"
	"github.com/skevetter/devpod/pkg/ide/fleet"
	"github.com/skevetter/devpod/pkg/ide/jetbrains"
	"github.com/skevetter/devpod/pkg/ide/jupyter"
	"github.com/skevetter/devpod/pkg/ide/openvscode"
	"github.com/skevetter/devpod/pkg/ide/rstudio"
	"github.com/skevetter/devpod/pkg/ide/vscode"
	"github.com/skevetter/devpod/pkg/provider"
)

type AllowedIDE struct {
	// Name of the IDE
	Name config.IDE `json:"name,omitempty"`
	// DisplayName is the name to show to the user
	DisplayName string `json:"displayName,omitempty"`
	// Options of the IDE
	Options ide.Options `json:"options,omitempty"`
	// Icon holds an image URL that will be displayed
	Icon string `json:"icon,omitempty"`
	// IconDark holds an image URL that will be displayed in dark mode
	IconDark string `json:"iconDark,omitempty"`
	// Experimental indicates that this IDE is experimental
	Experimental bool `json:"experimental,omitempty"`
	// Group this IDE belongs to, e.g. for navigation
	Group config.IDEGroup `json:"group,omitempty"`
}

var AllowedIDEs = []AllowedIDE{
	{
		Name:        config.IDENone,
		DisplayName: "None",
		Options:     map[string]ide.Option{},
		Icon:        config.WebsiteAssetsURL + "/none.svg",
		IconDark:    config.WebsiteAssetsURL + "/none_dark.svg",
		Group:       config.IDEGroupPrimary,
	},
	{
		Name:        config.IDEVSCode,
		DisplayName: "VS Code",
		Options:     vscode.Options,
		Icon:        config.WebsiteAssetsURL + "/vscode.svg",
		Group:       config.IDEGroupPrimary,
	},
	{
		Name:        config.IDEOpenVSCode,
		DisplayName: "VS Code Browser",
		Options:     openvscode.Options,
		Icon:        config.WebsiteAssetsURL + "/vscodebrowser.svg",
		Group:       config.IDEGroupPrimary,
	},
	{
		Name:         config.IDECursor,
		DisplayName:  "Cursor",
		Options:      vscode.Options,
		Icon:         config.WebsiteAssetsURL + "/cursor.svg",
		Experimental: true,
		Group:        config.IDEGroupPrimary,
	},
	{
		Name:         config.IDEZed,
		DisplayName:  "Zed",
		Options:      ide.Options{},
		Icon:         config.WebsiteAssetsURL + "/zed.svg",
		Experimental: true,
		Group:        config.IDEGroupPrimary,
	},
	{
		Name:         config.IDECodium,
		DisplayName:  "VSCodium",
		Options:      vscode.Options,
		Icon:         config.WebsiteAssetsURL + "/codium.svg",
		Experimental: true,
		Group:        config.IDEGroupPrimary,
	},
	{
		Name:        config.IDEIntellij,
		DisplayName: "IntelliJ IDEA",
		Options:     jetbrains.IntellijOptions,
		Icon:        config.WebsiteAssetsURL + "/intellij.svg",
		Group:       config.IDEGroupJetBrains,
	},
	{
		Name:        config.IDEPyCharm,
		DisplayName: "PyCharm",
		Options:     jetbrains.PyCharmOptions,
		Icon:        config.WebsiteAssetsURL + "/pycharm.svg",
		Group:       config.IDEGroupJetBrains,
	},
	{
		Name:        config.IDEPhpStorm,
		DisplayName: "PhpStorm",
		Options:     jetbrains.PhpStormOptions,
		Icon:        config.WebsiteAssetsURL + "/phpstorm.svg",
		Group:       config.IDEGroupJetBrains,
	},
	{
		Name:        config.IDERider,
		DisplayName: "Rider",
		Options:     jetbrains.RiderOptions,
		Icon:        config.WebsiteAssetsURL + "/rider.svg",
		Group:       config.IDEGroupJetBrains,
	},
	{
		Name:         config.IDEFleet,
		DisplayName:  "Fleet",
		Options:      fleet.Options,
		Icon:         config.WebsiteAssetsURL + "/fleet.svg",
		Experimental: true,
		Group:        config.IDEGroupJetBrains,
	},
	{
		Name:        config.IDEGoland,
		DisplayName: "GoLand",
		Options:     jetbrains.GolandOptions,
		Icon:        config.WebsiteAssetsURL + "/goland.svg",
		Group:       config.IDEGroupJetBrains,
	},
	{
		Name:        config.IDEWebStorm,
		DisplayName: "WebStorm",
		Options:     jetbrains.WebStormOptions,
		Icon:        config.WebsiteAssetsURL + "/webstorm.svg",
		Group:       config.IDEGroupJetBrains,
	},
	{
		Name:        config.IDERustRover,
		DisplayName: "RustRover",
		Options:     jetbrains.RustRoverOptions,
		Icon:        config.WebsiteAssetsURL + "/rustrover.svg",
		Group:       config.IDEGroupJetBrains,
	},
	{
		Name:        config.IDERubyMine,
		DisplayName: "RubyMine",
		Options:     jetbrains.RubyMineOptions,
		Icon:        config.WebsiteAssetsURL + "/rubymine.svg",
		Group:       config.IDEGroupJetBrains,
	},
	{
		Name:        config.IDECLion,
		DisplayName: "CLion",
		Options:     jetbrains.CLionOptions,
		Icon:        config.WebsiteAssetsURL + "/clion.svg",
		Group:       config.IDEGroupJetBrains,
	},
	{
		Name:        config.IDEDataSpell,
		DisplayName: "DataSpell",
		Options:     jetbrains.DataSpellOptions,
		Icon:        config.WebsiteAssetsURL + "/dataspell.svg",
		Group:       config.IDEGroupJetBrains,
	},
	{
		Name:         config.IDEJupyterNotebook,
		DisplayName:  "Jupyter Notebook",
		Options:      jupyter.Options,
		Icon:         config.WebsiteAssetsURL + "/jupyter.svg",
		IconDark:     config.WebsiteAssetsURL + "/jupyter_dark.svg",
		Experimental: true,
		Group:        config.IDEGroupOther,
	},
	{
		Name:         config.IDEVSCodeInsiders,
		DisplayName:  "VS Code Insiders",
		Options:      vscode.Options,
		Icon:         config.WebsiteAssetsURL + "/vscode_insiders.svg",
		Experimental: true,
		Group:        config.IDEGroupOther,
	},
	{
		Name:         config.IDEPositron,
		DisplayName:  "Positron",
		Options:      vscode.Options,
		Icon:         config.WebsiteAssetsURL + "/positron.svg",
		Experimental: true,
		Group:        config.IDEGroupOther,
	},
	{
		Name:         config.IDERStudio,
		DisplayName:  "RStudio Server",
		Options:      rstudio.Options,
		Icon:         config.WebsiteAssetsURL + "/rstudio.svg",
		Experimental: true,
		Group:        config.IDEGroupOther,
	},
	{
		Name:         config.IDEWindsurf,
		DisplayName:  "Windsurf Editor",
		Options:      vscode.Options,
		Icon:         config.WebsiteAssetsURL + "/windsurf.svg",
		Experimental: true,
		Group:        config.IDEGroupPrimary,
	},
	{
		Name:         config.IDEAntigravity,
		DisplayName:  "Google Antigravity",
		Options:      vscode.Options,
		Icon:         config.WebsiteAssetsURL + "/antigravity.svg",
		Experimental: true,
		Group:        config.IDEGroupPrimary,
	},
}

func RefreshIDEOptions(
	devPodConfig *config.Config,
	workspace *provider.Workspace,
	ide string,
	options []string,
) (*provider.Workspace, error) {
	ide = strings.ToLower(ide)
	if ide == "" {
		if workspace.IDE.Name != "" {
			ide = workspace.IDE.Name
		} else if devPodConfig.Current().DefaultIDE != "" {
			ide = devPodConfig.Current().DefaultIDE
		} else {
			ide = detect()
		}
	}

	// get ide options
	ideOptions, err := GetIDEOptions(ide)
	if err != nil {
		return nil, err
	}

	// get global options and set them as non user
	// provided.
	retValues := devPodConfig.IDEOptions(ide)
	for k, v := range retValues {
		retValues[k] = config.OptionValue{
			Value: v.Value,
		}
	}

	// get existing options
	if ide == workspace.IDE.Name {
		for k, v := range workspace.IDE.Options {
			if !v.UserProvided {
				continue
			}

			retValues[k] = v
		}
	}

	// get user options
	values, err := ParseOptions(options, ideOptions)
	if err != nil {
		return nil, fmt.Errorf("parse options: %w", err)
	}
	maps.Copy(retValues, values)

	// check if we need to modify workspace
	if workspace.IDE.Name != ide || !reflect.DeepEqual(workspace.IDE.Options, retValues) {
		workspace.IDE.Name = ide
		workspace.IDE.Options = retValues
		err = provider.SaveWorkspaceConfig(workspace)
		if err != nil {
			return nil, fmt.Errorf("save workspace: %w", err)
		}
	}

	return workspace, nil
}

func GetIDEOptions(ide string) (ide.Options, error) {
	var match *AllowedIDE
	for _, m := range AllowedIDEs {
		if string(m.Name) == ide {
			match = &m
			break
		}
	}
	if match == nil {
		allowedIDEArray := []string{}
		for _, a := range AllowedIDEs {
			allowedIDEArray = append(allowedIDEArray, string(a.Name))
		}

		return nil, fmt.Errorf("unrecognized ide '%s', please use one of: %v", ide, allowedIDEArray)
	}

	return match.Options, nil
}

func ParseOptions(options []string, ideOptions ide.Options) (map[string]config.OptionValue, error) {
	if ideOptions == nil {
		ideOptions = ide.Options{}
	}

	allowedOptions := []string{}
	for optionName := range ideOptions {
		allowedOptions = append(allowedOptions, optionName)
	}

	retMap := map[string]config.OptionValue{}
	for _, option := range options {
		splitted := strings.Split(option, "=")
		if len(splitted) == 1 {
			return nil, fmt.Errorf("invalid option '%s', expected format KEY=VALUE", option)
		}

		key := strings.ToUpper(strings.TrimSpace(splitted[0]))
		value := strings.Join(splitted[1:], "=")
		ideOption, ok := ideOptions[key]
		if !ok {
			return nil, fmt.Errorf(
				"invalid option '%s', allowed options are: %v",
				key,
				allowedOptions,
			)
		}

		if ideOption.ValidationPattern != "" {
			matcher, err := regexp.Compile(ideOption.ValidationPattern)
			if err != nil {
				return nil, err
			}

			if !matcher.MatchString(value) {
				if ideOption.ValidationMessage != "" {
					return nil, fmt.Errorf("%s", ideOption.ValidationMessage)
				}

				return nil, fmt.Errorf(
					"invalid value '%s' for option '%s', has to match the following regEx: %s",
					value,
					key,
					ideOption.ValidationPattern,
				)
			}
		}

		if len(ideOption.Enum) > 0 {
			found := slices.Contains(ideOption.Enum, value)
			if !found {
				return nil, fmt.Errorf(
					"invalid value '%s' for option '%s', has to match one of the following values: %v",
					value,
					key,
					ideOption.Enum,
				)
			}
		}

		retMap[key] = config.OptionValue{
			Value:        value,
			UserProvided: true,
		}
	}

	return retMap, nil
}

func detect() string {
	if command.Exists("code") {
		return string(config.IDEVSCode)
	}

	return string(config.IDEOpenVSCode)
}
