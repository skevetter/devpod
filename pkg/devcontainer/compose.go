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

type ensureContainerParams struct {
	parsedConfig        *config.SubstitutedConfig
	substitutionContext *config.SubstitutionContext
	project             *composetypes.Project
	composeHelper       *compose.ComposeHelper
	composeGlobalArgs   []string
	containerDetails    *config.ContainerDetails
	options             UpOptions
}

type buildPrepareParams struct {
	parsedConfig        *config.SubstitutedConfig
	substitutionContext *config.SubstitutionContext
	project             *composetypes.Project
	composeHelper       *compose.ComposeHelper
	composeService      *composetypes.ServiceConfig
	originalImageName   string
	composeGlobalArgs   *[]string
	options             UpOptions
}

type startContainerParams struct {
	parsedConfig        *config.SubstitutedConfig
	substitutionContext *config.SubstitutionContext
	project             *composetypes.Project
	composeHelper       *compose.ComposeHelper
	composeGlobalArgs   []string
	container           *config.ContainerDetails
	options             UpOptions
}

type tryStartProjectParams struct {
	parsedConfig  *config.SubstitutedConfig
	project       *composetypes.Project
	composeHelper *compose.ComposeHelper
	options       UpOptions
}

type composeUpOverrideParams struct {
	params                 buildPrepareParams
	mergedConfig           *config.MergedDevContainerConfig
	imageDetails           *config.ImageDetails
	metadataLabel          string
	overrideBuildImageName string
}

type buildInfoResult struct {
	imageBuildInfo     *config.ImageBuildInfo
	dockerfileContents string
	buildTarget        string
	err                error
}

func (r *runner) composeHelper() (*compose.ComposeHelper, error) {
	dockerDriver, ok := r.Driver.(driver.DockerDriver)
	if !ok {
		return nil, fmt.Errorf("docker compose is not supported by this provider, please choose a different one")
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

	_, _, composeGlobalArgs, err := r.dockerComposeProjectFiles(parsedConfig)
	if err != nil {
		return fmt.Errorf("get compose/env files: %w", err)
	}

	err = composeHelper.Stop(ctx, projectName, composeGlobalArgs)
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

	_, _, composeGlobalArgs, err := r.dockerComposeProjectFiles(parsedConfig)
	if err != nil {
		return fmt.Errorf("get compose/env files: %w", err)
	}

	err = composeHelper.Remove(ctx, projectName, composeGlobalArgs)
	if err != nil {
		return err
	}

	return nil
}

func (r *runner) dockerComposeProjectFiles(parsedConfig *config.SubstitutedConfig) ([]string, []string, []string, error) {
	envFiles, err := r.getEnvFiles()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("get env files: %w", err)
	}

	composeFiles, err := r.getDockerComposeFilePaths(parsedConfig, envFiles)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("get docker compose file paths: %w", err)
	}

	var args []string
	for _, configFile := range composeFiles {
		args = append(args, "-f", configFile)
	}

	for _, envFile := range envFiles {
		args = append(args, "--env-file", envFile)
	}

	return composeFiles, envFiles, args, nil
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

	composeFiles, envFiles, composeGlobalArgs, err := r.dockerComposeProjectFiles(parsedConfig)
	if err != nil {
		return nil, fmt.Errorf("get compose/env files: %w", err)
	}

	r.Log.Debugf("Loading docker compose project %+v", composeFiles)
	project, err := compose.LoadDockerComposeProject(ctx, composeFiles, envFiles)
	if err != nil {
		return nil, fmt.Errorf("load docker compose project: %w", err)
	}
	project.Name = composeHelper.GetProjectName(r.ID)
	r.Log.Debugf("Loaded project %s", project.Name)

	containerDetails, err := composeHelper.FindDevContainer(ctx, project.Name, parsedConfig.Config.Service)
	if err != nil {
		return nil, fmt.Errorf("find dev container: %w", err)
	}

	// does the container already exist or is it not running?
	if containerDetails == nil || containerDetails.State.Status != "running" || options.Recreate {
		params := ensureContainerParams{
			parsedConfig:        parsedConfig,
			substitutionContext: substitutionContext,
			project:             project,
			composeHelper:       composeHelper,
			composeGlobalArgs:   composeGlobalArgs,
			containerDetails:    containerDetails,
			options:             options,
		}
		var err error
		containerDetails, err = r.ensureContainerRunning(ctx, params)
		if err != nil {
			return nil, err
		}
	}

	imageMetadataConfig, err := metadata.GetImageMetadataFromContainer(containerDetails, substitutionContext, r.Log)
	if err != nil {
		return nil, fmt.Errorf("get image metadata from container: %w", err)
	}

	if dockerDriver, ok := r.Driver.(driver.DockerDriver); ok {
		err = dockerDriver.UpdateContainerUserUID(ctx, r.ID, parsedConfig.Config, r.Log.Writer(logrus.InfoLevel, false))
		if err != nil {
			r.Log.WithFields(logrus.Fields{"error": err}).Error("failed to update container user UID/GID")
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

// onlyRunServices appends the services defined in .devcontainer.json runServices to the upArgs
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

func (r *runner) getDockerComposeFilePaths(parsedConfig *config.SubstitutedConfig, envFiles []string) ([]string, error) {
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

func (r *runner) ensureContainerRunning(
	ctx context.Context,
	params ensureContainerParams,
) (*config.ContainerDetails, error) {
	tryParams := tryStartProjectParams{
		parsedConfig:  params.parsedConfig,
		project:       params.project,
		composeHelper: params.composeHelper,
		options:       params.options,
	}
	didStartProject, updatedDetails := r.tryStartExistingProject(ctx, tryParams)
	if didStartProject {
		return updatedDetails, nil
	}

	startParams := startContainerParams{
		parsedConfig:        params.parsedConfig,
		substitutionContext: params.substitutionContext,
		project:             params.project,
		composeHelper:       params.composeHelper,
		composeGlobalArgs:   params.composeGlobalArgs,
		container:           params.containerDetails,
		options:             params.options,
	}
	containerDetails, err := r.startContainer(ctx, startParams)
	if err != nil {
		return nil, fmt.Errorf("start container: %w", err)
	}
	if containerDetails == nil {
		return nil, fmt.Errorf("container not found after start")
	}
	return containerDetails, nil
}

func (r *runner) tryStartExistingProject(
	ctx context.Context,
	params tryStartProjectParams,
) (bool, *config.ContainerDetails) {
	existingProjectFiles, err := params.composeHelper.FindProjectFiles(ctx, params.project.Name)
	if err != nil {
		r.Log.Errorf("error finding project files: %s", err)
		return false, nil
	}

	if len(existingProjectFiles) == 0 || params.options.Recreate {
		return false, nil
	}

	if !r.allProjectFilesExist(existingProjectFiles) {
		return false, nil
	}

	return r.startProjectWithFiles(ctx, params, existingProjectFiles)
}

func (r *runner) allProjectFilesExist(files []string) bool {
	r.Log.Debugf("found existing project files: %s", files)
	for _, file := range files {
		if _, err := os.Stat(file); err != nil {
			r.Log.Warnf("project file %s does not exist anymore, recreating project", file)
			return false
		}
	}
	return true
}

func (r *runner) startProjectWithFiles(
	ctx context.Context,
	params tryStartProjectParams,
	existingProjectFiles []string,
) (bool, *config.ContainerDetails) {
	upArgs := []string{"--project-name", params.project.Name}
	for _, file := range existingProjectFiles {
		upArgs = append(upArgs, "-f", file)
	}
	upArgs = append(upArgs, "up", "-d")
	upArgs = r.onlyRunServices(upArgs, params.parsedConfig)

	writer := r.Log.Writer(logrus.InfoLevel, false)
	err := params.composeHelper.Run(ctx, upArgs, nil, writer, writer)
	if err != nil {
		r.Log.Errorf("error starting project: %s", err)
		return false, nil
	}

	details, err := params.composeHelper.FindDevContainer(
		ctx, params.project.Name, params.parsedConfig.Config.Service)
	if err != nil {
		r.Log.Errorf("error finding dev container: %s", err)
		return false, nil
	}

	return true, details
}

func (r *runner) tryRestorePersistedFiles(
	container *config.ContainerDetails,
	composeGlobalArgs *[]string,
) (bool, error) {
	labels := container.Config.Labels
	if labels[ConfigFilesLabel] == "" {
		return false, nil
	}

	configFiles := strings.Split(labels[ConfigFilesLabel], ",")
	return r.restoreFilesFromConfig(configFiles, composeGlobalArgs)
}

func (r *runner) restoreFilesFromConfig(configFiles []string, composeGlobalArgs *[]string) (bool, error) {
	buildFound, buildExists, buildFile, err := checkForPersistedFile(
		configFiles, FeaturesBuildOverrideFilePrefix)
	if err != nil {
		return false, fmt.Errorf("check for persisted build override: %w", err)
	}

	_, startExists, startFile, err := checkForPersistedFile(
		configFiles, FeaturesStartOverrideFilePrefix)
	if err != nil {
		return false, fmt.Errorf("check for persisted start override: %w", err)
	}

	if !r.shouldRestoreFiles(buildFound, buildExists, startExists) {
		return false, nil
	}

	if buildExists {
		*composeGlobalArgs = append(*composeGlobalArgs, "-f", buildFile)
	}
	if startExists {
		*composeGlobalArgs = append(*composeGlobalArgs, "-f", startFile)
	}

	return true, nil
}

func (r *runner) shouldRestoreFiles(buildFound, buildExists, startExists bool) bool {
	return (buildExists || !buildFound) && startExists
}

func (r *runner) buildAndPrepareCompose(
	ctx context.Context,
	params buildPrepareParams,
) error {
	overrideBuildImageName, overrideComposeBuildFilePath, imageMetadata, metadataLabel, err :=
		r.buildAndExtendDockerCompose(
			ctx, params.parsedConfig, params.substitutionContext, params.project, params.composeHelper,
			params.composeService, *params.composeGlobalArgs)
	if err != nil {
		return fmt.Errorf("build and extend docker-compose: %w", err)
	}

	if overrideComposeBuildFilePath != "" {
		*params.composeGlobalArgs = append(*params.composeGlobalArgs, "-f", overrideComposeBuildFilePath)
	}

	currentImageName := overrideBuildImageName
	if currentImageName == "" {
		currentImageName = params.originalImageName
	}

	imageDetails, err := r.inspectImage(ctx, currentImageName)
	if err != nil {
		return fmt.Errorf("inspect image: %w", err)
	}

	imageMetadata = r.mergeExtraConfig(imageMetadata, params.options)

	mergedConfig, err := config.MergeConfiguration(params.parsedConfig.Config, imageMetadata.Config)
	if err != nil {
		return fmt.Errorf("merge configuration: %w", err)
	}

	return r.addComposeUpOverride(composeUpOverrideParams{
		params:                 params,
		mergedConfig:           mergedConfig,
		imageDetails:           imageDetails,
		metadataLabel:          metadataLabel,
		overrideBuildImageName: overrideBuildImageName,
	})
}

func (r *runner) mergeExtraConfig(
	imageMetadata *config.ImageMetadataConfig,
	options UpOptions,
) *config.ImageMetadataConfig {
	if options.ExtraDevContainerPath == "" {
		return imageMetadata
	}

	if imageMetadata == nil {
		imageMetadata = &config.ImageMetadataConfig{}
	}

	extraConfig, err := config.ParseDevContainerJSONFile(options.ExtraDevContainerPath)
	if err == nil {
		config.AddConfigToImageMetadata(extraConfig, imageMetadata)
	}

	return imageMetadata
}

func (r *runner) addComposeUpOverride(overrideParams composeUpOverrideParams) error {
	additionalLabels := map[string]string{
		metadata.ImageMetadataLabel: overrideParams.metadataLabel,
		config.UserLabel:            overrideParams.imageDetails.Config.User,
	}
	overrideComposeUpFilePath, err := r.extendedDockerComposeUp(
		overrideParams.params.parsedConfig, overrideParams.mergedConfig,
		overrideParams.params.composeHelper, overrideParams.params.composeService,
		overrideParams.params.originalImageName, overrideParams.overrideBuildImageName,
		overrideParams.imageDetails, additionalLabels)
	if err != nil {
		return fmt.Errorf("extend docker-compose up: %w", err)
	}

	if overrideComposeUpFilePath != "" {
		*overrideParams.params.composeGlobalArgs = append(
			*overrideParams.params.composeGlobalArgs, "-f", overrideComposeUpFilePath)
	}

	return nil
}

func (r *runner) startContainer(ctx context.Context, params startContainerParams) (*config.ContainerDetails, error) {
	service := params.parsedConfig.Config.Service
	composeService, err := params.project.GetService(service)
	if err != nil {
		return nil, fmt.Errorf(
			"service %q configured in devcontainer.json not found in Docker Compose configuration",
			service)
	}

	originalImageName := composeService.Image
	if originalImageName == "" {
		originalImageName, err = params.composeHelper.GetDefaultImage(params.project.Name, service)
		if err != nil {
			return nil, fmt.Errorf("get default image: %w", err)
		}
	}

	if err := r.prepareContainer(ctx, params, composeService, originalImageName); err != nil {
		return nil, err
	}

	if err := r.recreateContainerIfNeeded(ctx, params); err != nil {
		return nil, err
	}

	return r.runComposeUp(ctx, params, composeService)
}

func (r *runner) prepareContainer(
	ctx context.Context,
	params startContainerParams,
	composeService composetypes.ServiceConfig,
	originalImageName string,
) error {
	if params.container != nil {
		didRestoreFromPersistedShare, err := r.tryRestorePersistedFiles(params.container, &params.composeGlobalArgs)
		if err != nil {
			return err
		}
		if didRestoreFromPersistedShare {
			return nil
		}
	}

	buildParams := buildPrepareParams{
		parsedConfig:        params.parsedConfig,
		substitutionContext: params.substitutionContext,
		project:             params.project,
		composeHelper:       params.composeHelper,
		composeService:      &composeService,
		originalImageName:   originalImageName,
		composeGlobalArgs:   &params.composeGlobalArgs,
		options:             params.options,
	}
	return r.buildAndPrepareCompose(ctx, buildParams)
}

func (r *runner) recreateContainerIfNeeded(ctx context.Context, params startContainerParams) error {
	if params.container == nil || !params.options.Recreate {
		return nil
	}

	r.Log.Debugf("deleting dev container %s due to --recreate", params.container.ID)

	if err := r.Driver.StopDevContainer(ctx, r.ID); err != nil {
		return fmt.Errorf("stop dev container: %w", err)
	}

	if err := r.Driver.DeleteDevContainer(ctx, r.ID); err != nil {
		return fmt.Errorf("delete dev container: %w", err)
	}

	return nil
}

func (r *runner) runComposeUp(
	ctx context.Context,
	params startContainerParams,
	composeService composetypes.ServiceConfig,
) (*config.ContainerDetails, error) {
	upArgs := []string{"--project-name", params.project.Name}
	upArgs = append(upArgs, params.composeGlobalArgs...)
	upArgs = append(upArgs, "up", "-d")
	if params.container != nil {
		upArgs = append(upArgs, "--no-recreate")
	}
	upArgs = r.onlyRunServices(upArgs, params.parsedConfig)

	writer := r.Log.Writer(logrus.InfoLevel, false)
	defer func() { _ = writer.Close() }()
	err := params.composeHelper.Run(ctx, upArgs, nil, writer, writer)
	if err != nil {
		return nil, fmt.Errorf("docker-compose run: %w", err)
	}

	containerDetails, err := params.composeHelper.FindDevContainer(
		ctx, params.project.Name, composeService.Name)
	if err != nil {
		return nil, fmt.Errorf("find dev container: %w", err)
	}

	return containerDetails, nil
}

// prepareComposeBuildInfo modifies a compose project's devcontainer Dockerfile to ensure it can be extended with features
// If an Image is specified instead of a Build, the metadata from the Image is used to populate the build info
func (r *runner) prepareComposeBuildInfo(ctx context.Context, subCtx *config.SubstitutionContext, composeService *composetypes.ServiceConfig, buildTarget string) (*config.ImageBuildInfo, string, string, error) {
	if composeService.Build != nil {
		result := r.prepareBuildFromDockerfile(subCtx, composeService)
		return result.imageBuildInfo, result.dockerfileContents, result.buildTarget, result.err
	}
	imageBuildInfo, err := r.getImageBuildInfoFromImage(ctx, subCtx, composeService.Image)
	return imageBuildInfo, "", "", err
}

func (r *runner) prepareBuildFromDockerfile(
	subCtx *config.SubstitutionContext,
	composeService *composetypes.ServiceConfig,
) buildInfoResult {
	dockerFilePath := composeService.Build.Dockerfile
	if !path.IsAbs(dockerFilePath) {
		dockerFilePath = filepath.Join(composeService.Build.Context, composeService.Build.Dockerfile)
	}

	originalDockerfile, err := os.ReadFile(dockerFilePath) // #nosec G304
	if err != nil {
		return buildInfoResult{err: err}
	}

	originalTarget := composeService.Build.Target
	if originalTarget != "" {
		imageBuildInfo, err := r.getImageBuildInfoFromDockerfile(
			subCtx, string(originalDockerfile),
			mappingToMap(composeService.Build.Args), originalTarget)
		return buildInfoResult{
			imageBuildInfo: imageBuildInfo,
			buildTarget:    originalTarget,
			err:            err,
		}
	}

	lastStageName, modifiedDockerfile, err := dockerfile.EnsureFinalStageName(
		string(originalDockerfile), config.DockerfileDefaultTarget)
	if err != nil {
		return buildInfoResult{err: err}
	}

	dockerfileContents := string(originalDockerfile)
	if modifiedDockerfile != "" {
		dockerfileContents = modifiedDockerfile
	}

	imageBuildInfo, err := r.getImageBuildInfoFromDockerfile(
		subCtx, string(originalDockerfile),
		mappingToMap(composeService.Build.Args), originalTarget)
	return buildInfoResult{
		imageBuildInfo:     imageBuildInfo,
		dockerfileContents: dockerfileContents,
		buildTarget:        lastStageName,
		err:                err,
	}
}

// This extends the build information for docker compose containers
func (r *runner) buildAndExtendDockerCompose(
	ctx context.Context,
	parsedConfig *config.SubstitutedConfig,
	substitutionContext *config.SubstitutionContext,
	project *composetypes.Project,
	composeHelper *compose.ComposeHelper,
	composeService *composetypes.ServiceConfig,
	globalArgs []string,
) (string, string, *config.ImageMetadataConfig, string, error) {
	var dockerFilePath, dockerfileContents, dockerComposeFilePath string
	var imageBuildInfo *config.ImageBuildInfo
	var err error

	buildImageName := composeService.Image
	// If Image is empty then we are building the dev container and use the default name docker-compose uses
	if buildImageName == "" {
		buildImageName = fmt.Sprintf("%s-%s", project.Name, composeService.Name)
	}
	buildTarget := "dev_container_auto_added_stage_label"

	// Determine base imageName for generated features build
	imageBuildInfo, dockerfileContents, buildTarget, err = r.prepareComposeBuildInfo(ctx, substitutionContext, composeService, buildTarget)
	if err != nil {
		return "", "", nil, "", err
	}

	extendImageBuildInfo, err := feature.GetExtendedBuildInfo(substitutionContext, imageBuildInfo, buildTarget, parsedConfig, r.Log, false)
	if err != nil {
		return "", "", nil, "", err
	}

	if extendImageBuildInfo != nil && extendImageBuildInfo.FeaturesBuildInfo != nil {
		// If the dockerfile is empty (because an Image was used) reference that image as the build target after the features / modified contents
		if dockerfileContents == "" {
			dockerfileContents = fmt.Sprintf("FROM %s AS %s\n", composeService.Image, buildTarget)
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
		err := os.WriteFile(extendedDockerfilePath, []byte(extendedDockerfileContent), 0600)
		if err != nil {
			return "", "", nil, "", fmt.Errorf("write Dockerfile with features: %w", err)
		}

		// Write the final docker-compose referencing the modified Dockerfile or Image
		dockerComposeFilePath, err = r.extendedDockerComposeBuild(
			composeService,
			extendedDockerfilePath,
			extendImageBuildInfo.FeaturesBuildInfo,
		)
		if err != nil {
			return buildImageName, "", nil, "", err
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
		return buildImageName, "", nil, "", err
	}

	imageMetadata, err := metadata.GetDevContainerMetadata(substitutionContext, imageBuildInfo.Metadata, parsedConfig, extendImageBuildInfo.Features)
	if err != nil {
		return buildImageName, "", nil, "", err
	}

	return buildImageName, dockerComposeFilePath, imageMetadata, extendImageBuildInfo.MetadataLabel, nil
}

func (r *runner) extendedDockerfile(featureBuildInfo *feature.BuildInfo, dockerfilePath, dockerfileContent string) (string, string) {
	// extra args?
	finalDockerfilePath := dockerfilePath
	finalDockerfileContent := dockerfileContent

	// get extended build info
	if featureBuildInfo != nil {
		// rewrite dockerfile path
		finalDockerfilePath = filepath.Join(featureBuildInfo.FeaturesFolder, "Dockerfile-with-features")

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

func (r *runner) extendedDockerComposeBuild(composeService *composetypes.ServiceConfig, dockerFilePath string, featuresBuildInfo *feature.BuildInfo) (string, error) {
	service := &composetypes.ServiceConfig{
		Name: composeService.Name,
		Build: &composetypes.BuildConfig{
			Dockerfile: dockerFilePath,
			Context:    filepath.Dir(featuresBuildInfo.FeaturesFolder),
		},
	}
	if composeService.Image != "" {
		service.Image = stripDigestFromImageRef(composeService.Image)
	}

	if composeService.Build != nil && composeService.Build.Target != "" {
		service.Build.Target = featuresBuildInfo.OverrideTarget
	}

	service.Build.Args = composetypes.NewMappingWithEquals([]string{"BUILDKIT_INLINE_CACHE=1"})
	for k, v := range featuresBuildInfo.BuildArgs {
		service.Build.Args[k] = &v
	}

	project := &composetypes.Project{}
	project.Services = map[string]composetypes.ServiceConfig{
		service.Name: *service,
	}

	dockerComposeFolder := getDockerComposeFolder(r.WorkspaceConfig.Origin)
	err := os.MkdirAll(dockerComposeFolder, 0755)
	if err != nil {
		return "", err
	}

	dockerComposeData, err := yaml.Marshal(project)
	if err != nil {
		return "", err
	}

	dockerComposePath := filepath.Join(dockerComposeFolder, fmt.Sprintf("%s-%d.yml", FeaturesBuildOverrideFilePrefix, time.Now().Second()))

	r.Log.Debugf(
		"Creating docker-compose build %s with content:\n %s",
		dockerComposePath,
		string(dockerComposeData),
	)

	err = os.WriteFile(dockerComposePath, dockerComposeData, 0600)
	if err != nil {
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
	dockerComposeUpProject := r.generateDockerComposeUpProject(parsedConfig, mergedConfig, composeHelper, composeService, originalImageName, overrideImageName, imageDetails, additionalLabels)
	dockerComposeData, err := yaml.Marshal(dockerComposeUpProject)
	if err != nil {
		return "", err
	}

	dockerComposeFolder := getDockerComposeFolder(r.WorkspaceConfig.Origin)
	err = os.MkdirAll(dockerComposeFolder, 0755)
	if err != nil {
		return "", err
	}

	dockerComposePath := filepath.Join(dockerComposeFolder, fmt.Sprintf("%s-%d.yml", FeaturesStartOverrideFilePrefix, time.Now().Second()))

	r.Log.Debugf(
		"Creating docker-compose up %s with content:\n %s",
		dockerComposePath,
		string(dockerComposeData),
	)

	err = os.WriteFile(dockerComposePath, dockerComposeData, 0600)
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

func (r *runner) configureGPUResources(parsedConfig *config.SubstitutedConfig, gpuSupportEnabled bool, overrideService *composetypes.ServiceConfig) {
	if parsedConfig.Config.HostRequirements != nil {
		enableGPU, warnIfMissing := parsedConfig.Config.HostRequirements.ShouldEnableGPU(gpuSupportEnabled)
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

func checkForPersistedFile(files []string, prefix string) (foundLabel bool, fileExists bool, filePath string, err error) {
	for _, file := range files {
		if !strings.HasPrefix(file, prefix) {
			continue
		}

		stat, err := os.Stat(file)
		if err == nil && stat.Mode().IsRegular() {
			return true, true, file, nil
		} else if os.IsNotExist(err) {
			return true, false, file, nil
		}
	}

	return false, false, "", nil
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
