package devcontainer

import (
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"

	"github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/skevetter/devpod/pkg/devcontainer/crane"
	"github.com/skevetter/devpod/pkg/language"
	provider2 "github.com/skevetter/devpod/pkg/provider"
)

func (r *runner) getRawConfig(options provider2.CLIOptions) (*config.DevContainerConfig, error) {
	if r.WorkspaceConfig.Workspace.DevContainerConfig != nil {
		rawParsedConfig := config.CloneDevContainerConfig(r.WorkspaceConfig.Workspace.DevContainerConfig)
		if r.WorkspaceConfig.Workspace.DevContainerPath != "" {
			rawParsedConfig.Origin = path.Join(filepath.ToSlash(r.LocalWorkspaceFolder), r.WorkspaceConfig.Workspace.DevContainerPath)
		} else {
			rawParsedConfig.Origin = path.Join(filepath.ToSlash(r.LocalWorkspaceFolder), ".devcontainer.devpod.json")
		}
		return rawParsedConfig, nil
	} else if r.WorkspaceConfig.Workspace.Source.Container != "" {
		return &config.DevContainerConfig{
			DevContainerConfigBase: config.DevContainerConfigBase{
				// Default workspace directory for containers
				// Upon inspecting the container, this would be updated to the correct folder, if found set
				WorkspaceFolder: "/",
			},
			RunningContainer: config.RunningContainer{
				ContainerID: r.WorkspaceConfig.Workspace.Source.Container,
			},
			Origin: "",
		}, nil
	} else if crane.ShouldUse(&options) {
		localWorkspaceFolder, err := crane.PullConfigFromSource(r.WorkspaceConfig, &options, r.Log)
		if err != nil {
			return nil, err
		}

		return config.ParseDevContainerJSON(
			localWorkspaceFolder,
			r.WorkspaceConfig.Workspace.DevContainerPath,
		)
	}

	localWorkspaceFolder := r.LocalWorkspaceFolder
	// if a subpath is specified, let's move to it

	if r.WorkspaceConfig.Workspace.Source.GitSubPath != "" {
		localWorkspaceFolder = filepath.Join(r.LocalWorkspaceFolder, r.WorkspaceConfig.Workspace.Source.GitSubPath)
	}

	// parse the devcontainer json
	var rawParsedConfig *config.DevContainerConfig
	var err error

	if options.DevContainerID != "" {
		// Use selector to find specific devcontainer by ID
		rawParsedConfig, err = config.ParseDevContainerJSONWithSelector(
			localWorkspaceFolder,
			r.WorkspaceConfig.Workspace.DevContainerPath,
			func(matches []string) (string, error) {
				for _, match := range matches {
					if filepath.Base(filepath.Dir(match)) == options.DevContainerID {
						return match, nil
					}
				}
				return "", fmt.Errorf("devcontainer with ID '%s' not found", options.DevContainerID)
			},
		)
	} else {
		rawParsedConfig, err = config.ParseDevContainerJSONWithSelector(
			localWorkspaceFolder,
			r.WorkspaceConfig.Workspace.DevContainerPath,
			func(matches []string) (string, error) {
				if len(matches) > 1 {
					ids, _ := config.ListDevContainerIDs(localWorkspaceFolder)
					return "", fmt.Errorf("multiple devcontainer configurations found. Use --devcontainer-id to select one: %v", ids)
				}
				return matches[0], nil
			},
		)
	}

	// We want to fail only in case of real errors, non-existing devcontainer.jon
	// will be gracefully handled by the auto-detection mechanism
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("parsing devcontainer.json: %w", err)
	} else if rawParsedConfig == nil {
		r.Log.Infof("Couldn't find a devcontainer.json")
		return r.getDefaultConfig(options)
	}
	return rawParsedConfig, nil
}

func (r *runner) getDefaultConfig(options provider2.CLIOptions) (*config.DevContainerConfig, error) {
	defaultConfig := &config.DevContainerConfig{}
	if options.FallbackImage != "" {
		r.Log.Infof("Using fallback image %s", options.FallbackImage)
		defaultConfig.ImageContainer = config.ImageContainer{
			Image: options.FallbackImage,
		}
	} else {
		r.Log.Infof("Try detecting project programming language...")
		defaultConfig = language.DefaultConfig(r.LocalWorkspaceFolder, r.Log)
	}

	defaultConfig.Origin = path.Join(filepath.ToSlash(r.LocalWorkspaceFolder), ".devcontainer.json")
	err := config.SaveDevContainerJSON(defaultConfig)
	if err != nil {
		return nil, fmt.Errorf("write default devcontainer.json: %w", err)
	}
	return defaultConfig, nil
}

func (r *runner) getSubstitutedConfig(options provider2.CLIOptions) (*config.SubstitutedConfig, *config.SubstitutionContext, error) {
	rawConfig, err := r.getRawConfig(options)
	if err != nil {
		return nil, nil, err
	}

	return r.substitute(options, rawConfig)
}

func (r *runner) substitute(
	options provider2.CLIOptions,
	rawParsedConfig *config.DevContainerConfig,
) (*config.SubstitutedConfig, *config.SubstitutionContext, error) {
	configFile := rawParsedConfig.Origin

	// get workspace folder within container
	workspaceMount, containerWorkspaceFolder := getWorkspace(
		r.LocalWorkspaceFolder,
		r.WorkspaceConfig.Workspace.ID,
		rawParsedConfig,
	)

	// merge InitEnv into environment for variable substitution
	env := config.ListToObject(os.Environ())
	if len(options.InitEnv) > 0 {
		initEnv := config.ListToObject(options.InitEnv)
		maps.Copy(env, initEnv)
	}

	substitutionContext := &config.SubstitutionContext{
		DevContainerID:           r.ID,
		LocalWorkspaceFolder:     r.LocalWorkspaceFolder,
		ContainerWorkspaceFolder: containerWorkspaceFolder,
		Env:                      env,

		WorkspaceMount: workspaceMount,
	}

	// substitute & load
	parsedConfig := &config.DevContainerConfig{}
	err := config.Substitute(substitutionContext, rawParsedConfig, parsedConfig)
	if err != nil {
		return nil, nil, err
	}
	if parsedConfig.WorkspaceFolder != "" {
		substitutionContext.ContainerWorkspaceFolder = parsedConfig.WorkspaceFolder
	}
	if parsedConfig.WorkspaceMount != "" {
		substitutionContext.WorkspaceMount = parsedConfig.WorkspaceMount
	}

	if options.DevContainerImage != "" {
		parsedConfig.Build = nil
		parsedConfig.Dockerfile = ""
		parsedConfig.DockerfileContainer = config.DockerfileContainer{}
		parsedConfig.ImageContainer = config.ImageContainer{Image: options.DevContainerImage}
	}

	parsedConfig.Origin = configFile
	return &config.SubstitutedConfig{
		Config: parsedConfig,
		Raw:    rawParsedConfig,
	}, substitutionContext, nil
}
