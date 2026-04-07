package devcontainer

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"

	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/pkg/compose"
	"github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/skevetter/devpod/pkg/devcontainer/feature"
	"github.com/skevetter/devpod/pkg/devcontainer/metadata"
	"github.com/skevetter/devpod/pkg/dockerfile"
	"github.com/skevetter/devpod/pkg/driver"
	"gopkg.in/yaml.v2"
)

const (
	ConfigFilesLabel                = "com.docker.compose.project.config_files"
	FeaturesBuildOverrideFilePrefix = "docker-compose.devcontainer.build"
	FeaturesStartOverrideFilePrefix = "docker-compose.devcontainer.containerFeatures"
)

type composeProjectFiles struct {
	composeFiles      []string
	envFiles          []string
	composeGlobalArgs []string
}

type composeBuildInfo struct {
	imageBuildInfo     *config.ImageBuildInfo
	dockerfileContents string
	buildTarget        string
}

type composeExtendResult struct {
	buildImageName       string
	composeBuildFilePath string
	imageMetadata        *config.ImageMetadataConfig
	metadataLabel        string
}

type persistedFileResult struct {
	foundLabel bool
	fileExists bool
	filePath   string
}

func (r *runner) composeHelper() (*compose.ComposeHelper, error) {
	dockerDriver, ok := r.Driver.(driver.DockerDriver)
	if !ok {
		return nil, fmt.Errorf(
			"docker compose is not supported by this provider, please choose a different one",
		)
	}

	return dockerDriver.ComposeHelper()
}

func (r *runner) stopDockerCompose(ctx context.Context, projectName string) error {
	composeHelper, err := r.composeHelper()
	if err != nil {
		return fmt.Errorf("find docker compose: %w", err)
	}

	parsedConfig, _, err := r.getSubstitutedConfig(r.WorkspaceConfig.CLIOptions)
	if err != nil {
		return fmt.Errorf("get parsed config: %w", err)
	}

	projFiles, err := r.dockerComposeProjectFiles(parsedConfig)
	if err != nil {
		return fmt.Errorf("get compose/env files: %w", err)
	}

	err = composeHelper.Stop(ctx, projectName, projFiles.composeGlobalArgs)
	if err != nil {
		return err
	}

	return nil
}

func (r *runner) deleteDockerCompose(ctx context.Context, projectName string) error {
	composeHelper, err := r.composeHelper()
	if err != nil {
		return fmt.Errorf("find docker compose: %w", err)
	}

	parsedConfig, _, err := r.getSubstitutedConfig(r.WorkspaceConfig.CLIOptions)
	if err != nil {
		return fmt.Errorf("get parsed config: %w", err)
	}

	projFiles, err := r.dockerComposeProjectFiles(parsedConfig)
	if err != nil {
		return fmt.Errorf("get compose/env files: %w", err)
	}

	err = composeHelper.Remove(ctx, projectName, projFiles.composeGlobalArgs)
	if err != nil {
		return err
	}

	return nil
}

func (r *runner) dockerComposeProjectFiles(
	parsedConfig *config.SubstitutedConfig,
) (composeProjectFiles, error) {
	envFiles, err := r.getEnvFiles()
	if err != nil {
		return composeProjectFiles{}, fmt.Errorf("get env files: %w", err)
	}

	composeFiles, err := r.getDockerComposeFilePaths(parsedConfig, envFiles)
	if err != nil {
		return composeProjectFiles{}, fmt.Errorf("get docker compose file paths: %w", err)
	}

	var args []string
	for _, configFile := range composeFiles {
		args = append(args, "-f", configFile)
	}

	for _, envFile := range envFiles {
		args = append(args, "--env-file", envFile)
	}

	return composeProjectFiles{
		composeFiles:      composeFiles,
		envFiles:          envFiles,
		composeGlobalArgs: args,
	}, nil
}

func (r *runner) runDockerCompose(
	ctx context.Context,
	parsedConfig *config.SubstitutedConfig,
	substitutionContext *config.SubstitutionContext,
	options UpOptions,
	timeout time.Duration,
) (*config.Result, error) {
	composeHelper, err := r.composeHelper()
	if err != nil {
		return nil, fmt.Errorf("find docker compose: %w", err)
	}

	projFiles, err := r.dockerComposeProjectFiles(parsedConfig)
	if err != nil {
		return nil, fmt.Errorf("get compose/env files: %w", err)
	}
	composeGlobalArgs := projFiles.composeGlobalArgs

	r.Log.Debugf("Loading docker compose project %+v", projFiles.composeFiles)
	project, err := compose.LoadDockerComposeProject(
		ctx,
		projFiles.composeFiles,
		projFiles.envFiles,
	)
	if err != nil {
		return nil, fmt.Errorf("load docker compose project: %w", err)
	}
	project.Name = composeHelper.GetProjectName(r.ID)
	r.Log.Debugf("Loaded project %s", project.Name)

	containerDetails, err := composeHelper.FindDevContainer(
		ctx,
		project.Name,
		parsedConfig.Config.Service,
	)
	if err != nil {
		return nil, fmt.Errorf("find dev container: %w", err)
	}

	// does the container already exist or is it not running?
	if containerDetails == nil || containerDetails.State.Status != "running" || options.Recreate {
		didStartProject := false
		// Try to find existing project first
		existingProjectFiles, err := composeHelper.FindProjectFiles(ctx, project.Name)
		if err != nil {
			r.Log.Errorf("Error finding project files: %s", err)
		} else if len(existingProjectFiles) > 0 && !options.Recreate {
			r.Log.Debugf("Found existing project files: %s", existingProjectFiles)
			// make sure all project files are still available
			for _, file := range existingProjectFiles {
				if _, err := os.Stat(file); err != nil {
					r.Log.Warnf("Project file %s does not exist anymore, recreating project", file)
					containerDetails = nil
					break
				}
			}

			// If project is found, we can call `up` with the project name
			// If it fails, fall back to rebuilding
			upArgs := []string{"--project-name", project.Name}
			for _, existingProjectFiles := range existingProjectFiles {
				upArgs = append(upArgs, "-f", existingProjectFiles)
			}
			upArgs = append(upArgs, "up", "-d")
			upArgs = r.onlyRunServices(upArgs, parsedConfig)

			// Run docker-compose
			writer := r.Log.Writer(logrus.InfoLevel, false)
			err = composeHelper.Run(ctx, upArgs, nil, writer, writer)
			if err != nil {
				r.Log.Errorf("Error starting project: %s", err)
			} else {
				// wait for running and get container details
				details, err := composeHelper.FindDevContainer(
					ctx,
					project.Name,
					parsedConfig.Config.Service,
				)
				if err != nil {
					r.Log.Errorf("Error finding dev container: %s", err)
				} else {
					containerDetails = details
					didStartProject = true
				}
			}
		}

		// Start container if not running
		if !didStartProject {
			containerDetails, err = r.startContainer(
				ctx,
				parsedConfig,
				substitutionContext,
				project,
				composeHelper,
				composeGlobalArgs,
				containerDetails,
				options,
			)
			if err != nil {
				return nil, fmt.Errorf("start container: %w", err)
			} else if containerDetails == nil {
				return nil, fmt.Errorf("couldn't find container after start")
			}
		}
	}

	imageMetadataConfig, err := metadata.GetImageMetadataFromContainer(
		containerDetails,
		substitutionContext,
		r.Log,
	)
	if err != nil {
		return nil, fmt.Errorf("get image metadata from container: %w", err)
	}

	if dockerDriver, ok := r.Driver.(driver.DockerDriver); ok {
		err = dockerDriver.UpdateContainerUserUID(
			ctx,
			r.ID,
			parsedConfig.Config,
			r.Log.Writer(logrus.InfoLevel, false),
		)
		if err != nil {
			r.Log.WithFields(logrus.Fields{"error": err}).
				Error("failed to update container user UID/GID")
			return nil, err
		}
	}

	if options.ExtraDevContainerPath != "" {
		if imageMetadataConfig == nil {
			imageMetadataConfig = &config.ImageMetadataConfig{}
		}
		extraConfig, err := config.ParseDevContainerJSONFile(options.ExtraDevContainerPath)
		if err != nil {
			return nil, err
		}
		config.AddConfigToImageMetadata(extraConfig, imageMetadataConfig)
	}

	mergedConfig, err := config.MergeConfiguration(parsedConfig.Config, imageMetadataConfig.Config)
	if err != nil {
		return nil, fmt.Errorf("merge config: %w", err)
	}

	// setup container
	return r.setupContainer(ctx, &setupContainerParams{
		rawConfig:           parsedConfig.Raw,
		containerDetails:    containerDetails,
		mergedConfig:        mergedConfig,
		substitutionContext: substitutionContext,
		timeout:             timeout,
	})
}

// onlyRunServices appends the services defined in .devcontainer.json runServices to the upArgs.
func (r *runner) onlyRunServices(upArgs []string, parsedConfig *config.SubstitutedConfig) []string {
	if len(parsedConfig.Config.RunServices) > 0 {
		// Run the main devcontainer
		upArgs = append(upArgs, parsedConfig.Config.Service)
		// Run the services defined in .devcontainer.json runServices
		for _, service := range parsedConfig.Config.RunServices {
			if service == parsedConfig.Config.Service {
				continue
			}
			upArgs = append(upArgs, service)
		}
	}
	return upArgs
}

func (r *runner) getDockerComposeFilePaths(
	parsedConfig *config.SubstitutedConfig,
	envFiles []string,
) ([]string, error) {
	configFileDir := filepath.Dir(parsedConfig.Config.Origin)

	// Use docker compose files from config
	var composeFiles []string
	if len(parsedConfig.Config.DockerComposeFile) > 0 {
		for _, composeFile := range parsedConfig.Config.DockerComposeFile {
			absPath := composeFile
			if !filepath.IsAbs(composeFile) {
				absPath = filepath.Join(configFileDir, composeFile)
			}
			composeFiles = append(composeFiles, absPath)
		}

		return composeFiles, nil
	}

	// Use docker compose files from $COMPOSE_FILE environment variable
	envComposeFile := os.Getenv("COMPOSE_FILE")

	// Load docker compose files from $COMPOSE_FILE in .env file
	if envComposeFile == "" {
		for _, envFile := range envFiles {
			env, err := godotenv.Read(envFile)
			if err != nil {
				return nil, err
			}

			if env["COMPOSE_FILE"] != "" {
				envComposeFile = env["COMPOSE_FILE"]
				break
			}
		}
	}

	if envComposeFile != "" {
		return filepath.SplitList(envComposeFile), nil
	}

	return nil, nil
}

func (r *runner) getEnvFiles() ([]string, error) {
	var envFiles []string
	envFile := path.Join(r.LocalWorkspaceFolder, ".env")
	envFileStat, err := os.Stat(envFile)
	if err == nil && envFileStat.Mode().IsRegular() {
		envFiles = append(envFiles, envFile)
	}
	return envFiles, nil
}

func (r *runner) startContainer(
	ctx context.Context,
	parsedConfig *config.SubstitutedConfig,
	substitutionContext *config.SubstitutionContext,
	project *composetypes.Project,
	composeHelper *compose.ComposeHelper,
	composeGlobalArgs []string,
	container *config.ContainerDetails,
	options UpOptions,
) (*config.ContainerDetails, error) {
	service := parsedConfig.Config.Service
	composeService, err := project.GetService(service)
	if err != nil {
		return nil, fmt.Errorf(
			"service '%s' configured in devcontainer.json not found in Docker Compose configuration",
			service,
		)
	}

	originalImageName := composeService.Image
	if originalImageName == "" {
		originalImageName, err = composeHelper.GetDefaultImage(project.Name, service)
		if err != nil {
			return nil, fmt.Errorf("get default image: %w", err)
		}
	}

	var didRestoreFromPersistedShare bool
	if container != nil {
		labels := container.Config.Labels
		if labels[ConfigFilesLabel] != "" {
			configFiles := strings.Split(labels[ConfigFilesLabel], ",")

			persistedBuildFile := checkForPersistedFile(
				configFiles,
				FeaturesBuildOverrideFilePrefix,
			)

			persistedStartFile := checkForPersistedFile(
				configFiles,
				FeaturesStartOverrideFilePrefix,
			)

			if (persistedBuildFile.fileExists || !persistedBuildFile.foundLabel) &&
				persistedStartFile.fileExists {
				didRestoreFromPersistedShare = true

				if persistedBuildFile.fileExists {
					composeGlobalArgs = append(composeGlobalArgs, "-f", persistedBuildFile.filePath)
				}

				if persistedStartFile.fileExists {
					composeGlobalArgs = append(composeGlobalArgs, "-f", persistedStartFile.filePath)
				}
			}
		}
	}

	if container == nil || !didRestoreFromPersistedShare {
		extendResult, err := r.buildAndExtendDockerCompose(
			ctx,
			parsedConfig,
			substitutionContext,
			project,
			composeHelper,
			&composeService,
			composeGlobalArgs,
		)
		if err != nil {
			return nil, fmt.Errorf("build and extend docker-compose: %w", err)
		}

		if extendResult.composeBuildFilePath != "" {
			composeGlobalArgs = append(composeGlobalArgs, "-f", extendResult.composeBuildFilePath)
		}

		currentImageName := extendResult.buildImageName
		if currentImageName == "" {
			currentImageName = originalImageName
		}

		imageDetails, err := r.inspectImage(ctx, currentImageName)
		if err != nil {
			return nil, fmt.Errorf("inspect image: %w", err)
		}

		if options.ExtraDevContainerPath != "" {
			if extendResult.imageMetadata == nil {
				extendResult.imageMetadata = &config.ImageMetadataConfig{}
			}
			extraConfig, err := config.ParseDevContainerJSONFile(options.ExtraDevContainerPath)
			if err != nil {
				return nil, err
			}
			config.AddConfigToImageMetadata(extraConfig, extendResult.imageMetadata)
		}

		mergedConfig, err := config.MergeConfiguration(
			parsedConfig.Config,
			extendResult.imageMetadata.Config,
		)
		if err != nil {
			return nil, fmt.Errorf("merge configuration: %w", err)
		}

		additionalLabels := map[string]string{
			metadata.ImageMetadataLabel: extendResult.metadataLabel,
			config.UserLabel:            imageDetails.Config.User,
		}
		overrideComposeUpFilePath, err := r.extendedDockerComposeUp(
			parsedConfig,
			mergedConfig,
			composeHelper,
			&composeService,
			originalImageName,
			extendResult.buildImageName,
			imageDetails,
			additionalLabels,
		)
		if err != nil {
			return nil, fmt.Errorf("extend docker-compose up: %w", err)
		}

		if overrideComposeUpFilePath != "" {
			composeGlobalArgs = append(composeGlobalArgs, "-f", overrideComposeUpFilePath)
		}
	}

	if container != nil && options.Recreate {
		r.Log.Debugf("Deleting dev container %s due to --recreate", container.ID)

		if err := r.Driver.StopDevContainer(ctx, r.ID); err != nil {
			return nil, fmt.Errorf("stop dev container: %w", err)
		}

		if err := r.Driver.DeleteDevContainer(ctx, r.ID); err != nil {
			return nil, fmt.Errorf("delete dev container: %w", err)
		}
	}

	upArgs := []string{"--project-name", project.Name}
	upArgs = append(upArgs, composeGlobalArgs...)
	upArgs = append(upArgs, "up", "-d")
	if container != nil {
		upArgs = append(upArgs, "--no-recreate")
	}
	upArgs = r.onlyRunServices(upArgs, parsedConfig)

	// start compose
	writer := r.Log.Writer(logrus.InfoLevel, false)
	defer func() { _ = writer.Close() }()
	err = composeHelper.Run(ctx, upArgs, nil, writer, writer)
	if err != nil {
		return nil, fmt.Errorf("docker-compose run: %w", err)
	}

	// TODO wait for started event?
	containerDetails, err := composeHelper.FindDevContainer(ctx, project.Name, composeService.Name)
	if err != nil {
		return nil, fmt.Errorf("find dev container: %w", err)
	}

	return containerDetails, nil
}

// prepareComposeBuildInfo modifies a compose project's devcontainer Dockerfile
// to ensure it can be extended with features. If an Image is specified instead
// of a Build, the metadata from the Image is used to populate the build info.
func (r *runner) prepareComposeBuildInfo(
	ctx context.Context,
	subCtx *config.SubstitutionContext,
	composeService *composetypes.ServiceConfig,
	buildTarget string,
) (composeBuildInfo, error) {
	var dockerFilePath, dockerfileContents string
	var imageBuildInfo *config.ImageBuildInfo
	var err error
	if composeService.Build != nil {
		// Read Dockerfile
		if path.IsAbs(composeService.Build.Dockerfile) {
			dockerFilePath = composeService.Build.Dockerfile
		} else {
			dockerFilePath = filepath.Join(
				composeService.Build.Context,
				composeService.Build.Dockerfile,
			)
		}

		originalDockerfile, err := os.ReadFile(dockerFilePath)
		if err != nil {
			return composeBuildInfo{}, err
		}

		// Determine build target. If a multi stage build is used, ensure it is
		// valid and modify the Dockerfile if necessary.
		originalTarget := composeService.Build.Target
		if originalTarget != "" {
			buildTarget = originalTarget
		} else {
			lastStageName, modifiedDockerfile, err := dockerfile.EnsureFinalStageName(
				string(originalDockerfile),
				config.DockerfileDefaultTarget,
			)
			if err != nil {
				return composeBuildInfo{}, err
			}

			buildTarget = lastStageName
			// Override Dockerfile if it was modified, otherwise use the original
			if modifiedDockerfile != "" {
				dockerfileContents = modifiedDockerfile
			} else {
				dockerfileContents = string(originalDockerfile)
			}
		}
		imageBuildInfo, err = r.getImageBuildInfoFromDockerfile(
			subCtx,
			string(originalDockerfile),
			mappingToMap(composeService.Build.Args),
			originalTarget,
		)
		if err != nil {
			return composeBuildInfo{}, err
		}
	} else {
		imageBuildInfo, err = r.getImageBuildInfoFromImage(ctx, subCtx, composeService.Image)
		if err != nil {
			return composeBuildInfo{}, err
		}
	}
	return composeBuildInfo{
		imageBuildInfo:     imageBuildInfo,
		dockerfileContents: dockerfileContents,
		buildTarget:        buildTarget,
	}, nil
}

// This extends the build information for docker compose containers.
func (r *runner) buildAndExtendDockerCompose(
	ctx context.Context,
	parsedConfig *config.SubstitutedConfig,
	substitutionContext *config.SubstitutionContext,
	project *composetypes.Project,
	composeHelper *compose.ComposeHelper,
	composeService *composetypes.ServiceConfig,
	globalArgs []string,
) (composeExtendResult, error) {
	var dockerFilePath, dockerfileContents, dockerComposeFilePath string
	var imageBuildInfo *config.ImageBuildInfo
	var err error

	buildTarget := "dev_container_auto_added_stage_label"

	// Determine base imageName for generated features build
	buildInfo, err := r.prepareComposeBuildInfo(
		ctx,
		substitutionContext,
		composeService,
		buildTarget,
	)
	if err != nil {
		return composeExtendResult{}, err
	}
	imageBuildInfo = buildInfo.imageBuildInfo
	dockerfileContents = buildInfo.dockerfileContents
	buildTarget = buildInfo.buildTarget

	extendImageBuildInfo, err := feature.GetExtendedBuildInfo(
		substitutionContext,
		imageBuildInfo,
		buildTarget,
		parsedConfig,
		r.Log,
		false,
	)
	if err != nil {
		return composeExtendResult{}, err
	}

	hasFeatures := extendImageBuildInfo != nil && extendImageBuildInfo.FeaturesBuildInfo != nil
	buildImageName, err := composeBuildImageName(
		composeHelper,
		project.Name,
		composeService,
		hasFeatures,
	)
	if err != nil {
		return composeExtendResult{}, err
	}

	if hasFeatures {
		// If the dockerfile is empty (because an Image was used), reference that
		// image as the build target after the features / modified contents.
		if dockerfileContents == "" {
			if composeService.Image == "" && composeService.Build == nil {
				return composeExtendResult{}, fmt.Errorf(
					"compose service %q has no image or build configuration",
					composeService.Name,
				)
			}
			sanitizedImage := strings.ReplaceAll(
				strings.ReplaceAll(composeService.Image, "\n", ""),
				"\r",
				"",
			)
			dockerfileContents = fmt.Sprintf("FROM %s AS %s\n", sanitizedImage, buildTarget)
		}

		// Write the final Dockerfile with features
		extendedDockerfilePath, extendedDockerfileContent := r.extendedDockerfile(
			extendImageBuildInfo.FeaturesBuildInfo,
			dockerFilePath,
			dockerfileContents,
		)

		r.Log.Debugf(
			"Creating extended Dockerfile %s with content: \n %s",
			extendedDockerfilePath,
			extendedDockerfileContent,
		)

		defer func() { _ = os.RemoveAll(filepath.Dir(extendedDockerfilePath)) }()

		// Write the final docker-compose referencing the modified Dockerfile or Image
		dockerComposeFilePath, err = r.extendedDockerComposeBuild(
			composeService,
			buildImageName,
			extendedDockerfilePath,
			extendedDockerfileContent,
			extendImageBuildInfo.FeaturesBuildInfo,
		)
		if err != nil {
			return composeExtendResult{buildImageName: buildImageName}, err
		}
	}

	// Prepare the docker-compose build arguments
	buildArgs := []string{"--project-name", project.Name}
	buildArgs = append(buildArgs, globalArgs...)
	if dockerComposeFilePath != "" {
		buildArgs = append(buildArgs, "-f", dockerComposeFilePath)
	}
	buildArgs = append(buildArgs, "build")
	if extendImageBuildInfo == nil {
		buildArgs = append(buildArgs, "--pull")
	}

	// Only run the services defined in .devcontainer.json runServices
	if len(parsedConfig.Config.RunServices) > 0 {
		buildArgs = append(buildArgs, composeService.Name)
		for _, service := range parsedConfig.Config.RunServices {
			if service == composeService.Name {
				continue
			}
			buildArgs = append(buildArgs, service)
		}
	}

	// build image
	writer := r.Log.Writer(logrus.InfoLevel, false)
	defer func() { _ = writer.Close() }()
	r.Log.Debugf("Run %s %s", composeHelper.Command, strings.Join(buildArgs, " "))
	err = composeHelper.Run(ctx, buildArgs, nil, writer, writer)
	if err != nil {
		return composeExtendResult{buildImageName: buildImageName}, err
	}

	imageMetadata, err := metadata.GetDevContainerMetadata(
		substitutionContext,
		imageBuildInfo.Metadata,
		parsedConfig,
		extendImageBuildInfo.Features,
	)
	if err != nil {
		return composeExtendResult{buildImageName: buildImageName}, err
	}

	return composeExtendResult{
		buildImageName:       buildImageName,
		composeBuildFilePath: dockerComposeFilePath,
		imageMetadata:        imageMetadata,
		metadataLabel:        extendImageBuildInfo.MetadataLabel,
	}, nil
}

func (r *runner) extendedDockerfile(
	featureBuildInfo *feature.BuildInfo,
	dockerfilePath, dockerfileContent string,
) (string, string) {
	// extra args?
	finalDockerfilePath := dockerfilePath
	finalDockerfileContent := dockerfileContent

	// get extended build info
	if featureBuildInfo != nil {
		// rewrite dockerfile path
		finalDockerfilePath = filepath.Join(
			featureBuildInfo.FeaturesFolder,
			"Dockerfile-with-features",
		)

		// rewrite dockerfile
		finalDockerfileContent = dockerfile.RemoveSyntaxVersion(dockerfileContent)
		finalDockerfileContent = strings.TrimSpace(strings.Join([]string{
			featureBuildInfo.DockerfilePrefixContent,
			strings.TrimSpace(finalDockerfileContent),
			featureBuildInfo.DockerfileContent,
		}, "\n"))
	}

	return finalDockerfilePath, finalDockerfileContent
}

func (r *runner) setBuildPathsForContext(
	originalContext, dockerFilePath, dockerfileContent, featuresFolder string,
) (relDockerfilePath string, modifiedDockerfileContent string, err error) {
	absBuildContext, err := filepath.Abs(originalContext)
	if err != nil {
		return "", "", err
	}

	absDockerFilePath, err := filepath.Abs(dockerFilePath)
	if err != nil {
		return "", "", err
	}
	relDockerfilePath, err = filepath.Rel(absBuildContext, absDockerFilePath)
	if err != nil {
		return "", "", err
	}

	absFeatureFolder, err := filepath.Abs(featuresFolder)
	if err != nil {
		return "", "", err
	}
	relFeaturePath, err := filepath.Rel(absBuildContext, absFeatureFolder)
	if err != nil {
		return "", "", err
	}

	// Rewrite COPY/ADD directives that reference the features folder to use the relative path
	// from the custom build context. This ensures that the features folder is referenced in the
	// Dockerfile.
	pattern := regexp.MustCompile(
		`(COPY|ADD)(\s+)\./` + regexp.QuoteMeta(config.DevPodContextFeatureFolder) + `/`,
	)
	modifiedDockerfileContent = pattern.ReplaceAllString(
		dockerfileContent,
		"${1}${2}./"+filepath.ToSlash(relFeaturePath)+"/",
	)

	return relDockerfilePath, modifiedDockerfileContent, nil
}

type buildContextResult struct {
	context                 string
	dockerfilePathInContext string
	dockerfileContent       string
}

func (r *runner) extendedDockerComposeBuild(
	composeService *composetypes.ServiceConfig,
	buildImageName string,
	dockerFilePath string,
	dockerfileContent string,
	featuresBuildInfo *feature.BuildInfo,
) (string, error) {
	result, err := r.prepareBuildContext(
		composeService, dockerFilePath, dockerfileContent, featuresBuildInfo,
	)
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(dockerFilePath, []byte(result.dockerfileContent), 0o600); err != nil {
		return "", err
	}

	service := r.createComposeService(
		composeService,
		buildImageName,
		result.dockerfilePathInContext,
		result.context,
		featuresBuildInfo,
	)
	return r.writeComposeFile(service)
}

func (r *runner) prepareBuildContext(
	composeService *composetypes.ServiceConfig,
	dockerFilePath, dockerfileContent string,
	featuresBuildInfo *feature.BuildInfo,
) (*buildContextResult, error) {
	buildContext := filepath.Dir(featuresBuildInfo.FeaturesFolder)
	relDockerFilePath, err := filepath.Rel(buildContext, dockerFilePath)
	if err != nil {
		return nil, err
	}

	result := &buildContextResult{
		context:                 buildContext,
		dockerfilePathInContext: relDockerFilePath,
		dockerfileContent:       dockerfileContent,
	}

	if composeService.Build != nil && composeService.Build.Context != "" {
		relDockerFilePath, modifiedDockerfileContent, err := r.setBuildPathsForContext(
			composeService.Build.Context,
			dockerFilePath,
			dockerfileContent,
			featuresBuildInfo.FeaturesFolder,
		)
		if err != nil {
			return nil, err
		}
		r.Log.Debugf(
			"modified Dockerfile path in context to %s and content for extended compose build context %s",
			relDockerFilePath,
			composeService.Build.Context,
		)
		result.context = composeService.Build.Context
		result.dockerfilePathInContext = relDockerFilePath
		result.dockerfileContent = modifiedDockerfileContent
	}

	return result, nil
}

func (r *runner) createComposeService(
	composeService *composetypes.ServiceConfig,
	buildImageName string,
	dockerfilePathInContext, buildContext string,
	featuresBuildInfo *feature.BuildInfo,
) *composetypes.ServiceConfig {
	service := &composetypes.ServiceConfig{
		Name: composeService.Name,
		Build: &composetypes.BuildConfig{
			Dockerfile: dockerfilePathInContext,
			Context:    buildContext,
		},
	}
	if buildImageName != "" {
		service.Image = stripDigestFromImageRef(buildImageName)
	}

	if composeService.Build != nil && composeService.Build.Target != "" {
		service.Build.Target = featuresBuildInfo.OverrideTarget
	}

	service.Build.Args = composetypes.NewMappingWithEquals([]string{"BUILDKIT_INLINE_CACHE=1"})
	for k, v := range featuresBuildInfo.BuildArgs {
		service.Build.Args[k] = &v
	}

	return service
}

func composeBuildImageName(
	composeHelper *compose.ComposeHelper,
	projectName string,
	composeService *composetypes.ServiceConfig,
	hasFeatures bool,
) (string, error) {
	if hasFeatures && composeService.Image != "" && composeService.Build == nil {
		return composeHelper.GetDefaultImage(projectName, composeService.Name)
	}

	if composeService.Image != "" {
		return composeService.Image, nil
	}

	return composeHelper.GetDefaultImage(projectName, composeService.Name)
}

func (r *runner) writeComposeFile(service *composetypes.ServiceConfig) (string, error) {
	project := &composetypes.Project{
		Services: map[string]composetypes.ServiceConfig{
			service.Name: *service,
		},
	}

	dockerComposeFolder := getDockerComposeFolder(r.WorkspaceConfig.Origin)
	if err := os.MkdirAll(dockerComposeFolder, 0o750); err != nil {
		return "", err
	}

	dockerComposeData, err := yaml.Marshal(project)
	if err != nil {
		return "", err
	}

	dockerComposePath := filepath.Join(
		dockerComposeFolder,
		fmt.Sprintf("%s-%d.yml", FeaturesBuildOverrideFilePrefix, time.Now().Second()),
	)

	r.Log.Debugf(
		"Creating docker-compose build %s with content:\n %s",
		dockerComposePath,
		string(dockerComposeData),
	)

	if err := os.WriteFile(dockerComposePath, dockerComposeData, 0o600); err != nil {
		return "", err
	}

	return dockerComposePath, nil
}

func stripDigestFromImageRef(imageRef string) string {
	baseRef, _, found := strings.Cut(imageRef, "@")
	if !found {
		return imageRef
	}

	return baseRef
}

func (r *runner) extendedDockerComposeUp(
	parsedConfig *config.SubstitutedConfig,
	mergedConfig *config.MergedDevContainerConfig,
	composeHelper *compose.ComposeHelper,
	composeService *composetypes.ServiceConfig,
	originalImageName,
	overrideImageName string,
	imageDetails *config.ImageDetails,
	additionalLabels map[string]string,
) (string, error) {
	dockerComposeUpProject := r.generateDockerComposeUpProject(
		parsedConfig,
		mergedConfig,
		composeHelper,
		composeService,
		originalImageName,
		overrideImageName,
		imageDetails,
		additionalLabels,
	)
	dockerComposeData, err := yaml.Marshal(dockerComposeUpProject)
	if err != nil {
		return "", err
	}

	dockerComposeFolder := getDockerComposeFolder(r.WorkspaceConfig.Origin)
	err = os.MkdirAll(dockerComposeFolder, 0o750)
	if err != nil {
		return "", err
	}

	dockerComposePath := filepath.Join(
		dockerComposeFolder,
		fmt.Sprintf("%s-%d.yml", FeaturesStartOverrideFilePrefix, time.Now().Second()),
	)

	r.Log.Debugf(
		"Creating docker-compose up %s with content:\n %s",
		dockerComposePath,
		string(dockerComposeData),
	)

	err = os.WriteFile(dockerComposePath, dockerComposeData, 0o600)
	if err != nil {
		return "", err
	}
	return dockerComposePath, nil
}

func (r *runner) generateDockerComposeUpProject(
	parsedConfig *config.SubstitutedConfig,
	mergedConfig *config.MergedDevContainerConfig,
	composeHelper *compose.ComposeHelper,
	composeService *composetypes.ServiceConfig,
	originalImageName,
	overrideImageName string,
	imageDetails *config.ImageDetails,
	additionalLabels map[string]string,
) *composetypes.Project {
	// Configure overridden service
	userEntrypoint := composeService.Entrypoint
	userCommand := composeService.Command
	if mergedConfig.OverrideCommand != nil && *mergedConfig.OverrideCommand {
		userEntrypoint = []string{}
		userCommand = []string{}
	} else {
		if len(userEntrypoint) == 0 {
			userEntrypoint = imageDetails.Config.Entrypoint
		}

		if len(userCommand) == 0 {
			userCommand = imageDetails.Config.Cmd
		}
	}

	entrypoint := composetypes.ShellCommand{
		"/bin/sh",
		"-c",
		`echo Container started
trap "exit 0" 15
` + strings.Join(mergedConfig.Entrypoints, "\n") + `
exec "$$@"
` + DefaultEntrypoint,
		"-",
	}
	entrypoint = append(entrypoint, userEntrypoint...)

	labels := composetypes.Labels{
		config.DockerIDLabel: r.ID,
	}
	for k, v := range additionalLabels {
		// Escape $ and ' to prevent substituting local environment variables!
		label := regexp.MustCompile(`\$`).ReplaceAllString(v, "$$$$")
		label = regexp.MustCompile(`'`).ReplaceAllString(label, `\'\'`)
		labels.Add(k, label)
	}

	overrideService := &composetypes.ServiceConfig{
		Name:        composeService.Name,
		Entrypoint:  entrypoint,
		Environment: mappingFromMap(mergedConfig.ContainerEnv),
		Init:        mergedConfig.Init,
		CapAdd:      mergedConfig.CapAdd,
		SecurityOpt: mergedConfig.SecurityOpt,
		Labels:      labels,
	}

	if originalImageName != overrideImageName {
		overrideService.Image = overrideImageName
	}

	if !reflect.DeepEqual(userCommand, composeService.Command) {
		overrideService.Command = userCommand
	}

	if mergedConfig.ContainerUser != "" {
		overrideService.User = mergedConfig.ContainerUser
	}

	if mergedConfig.Privileged != nil {
		overrideService.Privileged = *mergedConfig.Privileged
	}

	gpuSupportEnabled, _ := composeHelper.Docker.GPUSupportEnabled()
	r.configureGPUResources(parsedConfig, gpuSupportEnabled, overrideService)

	for _, mount := range mergedConfig.Mounts {
		overrideService.Volumes = append(overrideService.Volumes, composetypes.ServiceVolumeConfig{
			Type:   mount.Type,
			Source: mount.Source,
			Target: mount.Target,
		})
	}

	project := &composetypes.Project{}
	project.Services = map[string]composetypes.ServiceConfig{
		overrideService.Name: *overrideService,
	}

	// Configure volumes
	var volumeMounts []composetypes.VolumeConfig
	for _, m := range mergedConfig.Mounts {
		if m.Type == "volume" {
			volumeMounts = append(volumeMounts, composetypes.VolumeConfig{
				Name:     m.Source,
				External: composetypes.External(m.External),
			})
		}
	}

	if len(volumeMounts) > 0 {
		project.Volumes = map[string]composetypes.VolumeConfig{}
	}
	for _, volumeMount := range volumeMounts {
		project.Volumes[volumeMount.Name] = volumeMount
	}

	return project
}

func (r *runner) configureGPUResources(
	parsedConfig *config.SubstitutedConfig,
	gpuSupportEnabled bool,
	overrideService *composetypes.ServiceConfig,
) {
	if parsedConfig.Config.HostRequirements != nil {
		enableGPU, warnIfMissing := parsedConfig.Config.HostRequirements.ShouldEnableGPU(
			gpuSupportEnabled,
		)
		if enableGPU {
			overrideService.Deploy = &composetypes.DeployConfig{
				Resources: composetypes.Resources{
					Reservations: &composetypes.Resource{
						Devices: []composetypes.DeviceRequest{
							{
								Capabilities: []string{"gpu"},
							},
						},
					},
				},
			}
		}
		if warnIfMissing {
			r.Log.Warn("GPU required but not available on host")
		}
	}
}

func checkForPersistedFile(
	files []string,
	prefix string,
) persistedFileResult {
	for _, file := range files {
		if !strings.HasPrefix(filepath.Base(file), prefix) {
			continue
		}

		stat, err := os.Stat(file)
		if err == nil && stat.Mode().IsRegular() {
			return persistedFileResult{foundLabel: true, fileExists: true, filePath: file}
		} else if os.IsNotExist(err) {
			return persistedFileResult{foundLabel: true, fileExists: false, filePath: file}
		}
	}

	return persistedFileResult{}
}

func getDockerComposeFolder(workspaceOriginFolder string) string {
	return filepath.Join(workspaceOriginFolder, ".docker-compose")
}

func mappingFromMap(m map[string]string) composetypes.MappingWithEquals {
	if len(m) == 0 {
		return nil
	}

	var values []string
	for k, v := range m {
		values = append(values, k+"="+v)
	}
	return composetypes.NewMappingWithEquals(values)
}

func mappingToMap(mapping composetypes.MappingWithEquals) map[string]string {
	ret := map[string]string{}
	for k, v := range mapping {
		ret[k] = *v
	}
	return ret
}

func isDockerComposeConfig(config *config.DevContainerConfig) bool {
	return len(config.DockerComposeFile) > 0
}
