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

type buildExtendParams struct {
	parsedConfig        *config.SubstitutedConfig
	substitutionContext *config.SubstitutionContext
	project             *composetypes.Project
	composeHelper       *compose.ComposeHelper
	composeService      *composetypes.ServiceConfig
	globalArgs          []string
}

type runComposeParams struct {
	parsedConfig        *config.SubstitutedConfig
	substitutionContext *config.SubstitutionContext
	options             UpOptions
	timeout             time.Duration
}

type composeUpParams struct {
	parsedConfig      *config.SubstitutedConfig
	mergedConfig      *config.MergedDevContainerConfig
	composeHelper     *compose.ComposeHelper
	composeService    *composetypes.ServiceConfig
	originalImageName string
	overrideImageName string
	imageDetails      *config.ImageDetails
	additionalLabels  map[string]string
}

type composeProjectFiles struct {
	composeFiles      []string
	envFiles          []string
	composeGlobalArgs []string
}

type composeProjectResult struct {
	composeHelper     *compose.ComposeHelper
	project           *composetypes.Project
	composeGlobalArgs []string
}

type buildExtendResult struct {
	buildImageName  string
	composeFilePath string
	imageMetadata   *config.ImageMetadataConfig
	metadataLabel   string
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

	projectFiles := r.dockerComposeProjectFiles(parsedConfig)
	composeGlobalArgs := projectFiles.composeGlobalArgs

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

	projectFiles := r.dockerComposeProjectFiles(parsedConfig)
	composeGlobalArgs := projectFiles.composeGlobalArgs

	err = composeHelper.Remove(ctx, projectName, composeGlobalArgs)
	if err != nil {
		return err
	}

	return nil
}

func (r *runner) dockerComposeProjectFiles(parsedConfig *config.SubstitutedConfig) *composeProjectFiles {
	envFiles := r.getEnvFiles()

	composeFiles := r.getDockerComposeFilePaths(parsedConfig, envFiles)

	var args []string
	for _, configFile := range composeFiles {
		args = append(args, "-f", configFile)
	}

	for _, envFile := range envFiles {
		args = append(args, "--env-file", envFile)
	}

	return &composeProjectFiles{
		composeFiles:      composeFiles,
		envFiles:          envFiles,
		composeGlobalArgs: args,
	}
}

func (r *runner) runDockerCompose(
	ctx context.Context,
	params runComposeParams,
) (*config.Result, error) {
	projectResult, err := r.loadComposeProject(ctx, params.parsedConfig)
	if err != nil {
		return nil, err
	}

	containerDetails, err := r.ensureComposeContainer(ctx, projectResult, params)
	if err != nil {
		return nil, err
	}

	imageMetadataConfig, err := r.getImageMetadata(ctx, containerDetails, params)
	if err != nil {
		return nil, err
	}

	mergedConfig, err := config.MergeConfiguration(params.parsedConfig.Config, imageMetadataConfig.Config)
	if err != nil {
		return nil, fmt.Errorf("merge config: %w", err)
	}

	return r.setupContainer(ctx, &setupContainerParams{
		rawConfig:           params.parsedConfig.Raw,
		containerDetails:    containerDetails,
		mergedConfig:        mergedConfig,
		substitutionContext: params.substitutionContext,
		timeout:             params.timeout,
	})
}

func (r *runner) loadComposeProject(
	ctx context.Context,
	parsedConfig *config.SubstitutedConfig,
) (*composeProjectResult, error) {
	composeHelper, err := r.composeHelper()
	if err != nil {
		return nil, fmt.Errorf("find docker compose: %w", err)
	}

	projectFiles := r.dockerComposeProjectFiles(parsedConfig)

	r.Log.Debugf("Loading docker compose project %+v", projectFiles.composeFiles)
	project, err := compose.LoadDockerComposeProject(ctx, projectFiles.composeFiles, projectFiles.envFiles)
	if err != nil {
		return nil, fmt.Errorf("load docker compose project: %w", err)
	}
	project.Name = composeHelper.GetProjectName(r.ID)
	r.Log.Debugf("Loaded project %s", project.Name)

	return &composeProjectResult{
		composeHelper:     composeHelper,
		project:           project,
		composeGlobalArgs: projectFiles.composeGlobalArgs,
	}, nil
}

func (r *runner) ensureComposeContainer(
	ctx context.Context,
	projectResult *composeProjectResult,
	params runComposeParams,
) (*config.ContainerDetails, error) {
	containerDetails, err := projectResult.composeHelper.FindDevContainer(
		ctx, projectResult.project.Name, params.parsedConfig.Config.Service)
	if err != nil {
		return nil, fmt.Errorf("find dev container: %w", err)
	}

	if containerDetails == nil || containerDetails.State.Status != "running" || params.options.Recreate {
		ensureParams := ensureContainerParams{
			parsedConfig:        params.parsedConfig,
			substitutionContext: params.substitutionContext,
			project:             projectResult.project,
			composeHelper:       projectResult.composeHelper,
			composeGlobalArgs:   projectResult.composeGlobalArgs,
			containerDetails:    containerDetails,
			options:             params.options,
		}
		return r.ensureContainerRunning(ctx, ensureParams)
	}

	return containerDetails, nil
}

func (r *runner) getImageMetadata(
	ctx context.Context,
	containerDetails *config.ContainerDetails,
	params runComposeParams,
) (*config.ImageMetadataConfig, error) {
	imageMetadataConfig, err := metadata.GetImageMetadataFromContainer(containerDetails, params.substitutionContext, r.Log)
	if err != nil {
		return nil, fmt.Errorf("get image metadata from container: %w", err)
	}

	if dockerDriver, ok := r.Driver.(driver.DockerDriver); ok {
		err = dockerDriver.UpdateContainerUserUID(
			ctx, r.ID, params.parsedConfig.Config, r.Log.Writer(logrus.InfoLevel, false))
		if err != nil {
			r.Log.WithFields(logrus.Fields{"error": err}).Error("failed to update container user UID/GID")
			return nil, err
		}
	}

	if params.options.ExtraDevContainerPath != "" {
		if imageMetadataConfig == nil {
			imageMetadataConfig = &config.ImageMetadataConfig{}
		}
		extraConfig, err := config.ParseDevContainerJSONFile(params.options.ExtraDevContainerPath)
		if err != nil {
			return nil, err
		}
		config.AddConfigToImageMetadata(extraConfig, imageMetadataConfig)
	}

	return imageMetadataConfig, nil
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
) []string {
	if len(parsedConfig.Config.DockerComposeFile) > 0 {
		return r.getAbsoluteComposePaths(parsedConfig)
	}

	envComposeFile := r.getComposeFileFromEnv(envFiles)
	if envComposeFile != "" {
		return filepath.SplitList(envComposeFile)
	}

	return nil
}

func (r *runner) getAbsoluteComposePaths(parsedConfig *config.SubstitutedConfig) []string {
	configFileDir := filepath.Dir(parsedConfig.Config.Origin)
	var composeFiles []string
	for _, composeFile := range parsedConfig.Config.DockerComposeFile {
		absPath := composeFile
		if !filepath.IsAbs(composeFile) {
			absPath = filepath.Join(configFileDir, composeFile)
		}
		composeFiles = append(composeFiles, absPath)
	}
	return composeFiles
}

func (r *runner) getComposeFileFromEnv(envFiles []string) string {
	envComposeFile := os.Getenv("COMPOSE_FILE")
	if envComposeFile != "" {
		return envComposeFile
	}

	for _, envFile := range envFiles {
		env, err := godotenv.Read(envFile)
		if err != nil {
			continue
		}
		if env["COMPOSE_FILE"] != "" {
			return env["COMPOSE_FILE"]
		}
	}
	return ""
}

func (r *runner) getEnvFiles() []string {
	var envFiles []string
	envFile := path.Join(r.LocalWorkspaceFolder, ".env")
	envFileStat, err := os.Stat(envFile)
	if err == nil && envFileStat.Mode().IsRegular() {
		envFiles = append(envFiles, envFile)
	}
	return envFiles
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

	// Filter out generated override files that may have been cleaned up
	validFiles := r.filterValidProjectFiles(existingProjectFiles)
	if len(validFiles) == 0 {
		return false, nil
	}

	return r.startProjectWithFiles(ctx, params, validFiles)
}

func (r *runner) filterValidProjectFiles(files []string) []string {
	r.Log.Debugf("found existing project files: %s", files)
	var validFiles []string
	for _, file := range files {
		// Skip generated override files that may have been cleaned up
		if strings.Contains(file, FeaturesBuildOverrideFilePrefix) ||
			strings.Contains(file, FeaturesStartOverrideFilePrefix) {
			r.Log.Debugf("skipping generated override file: %s", file)
			continue
		}
		if _, err := os.Stat(file); err != nil {
			r.Log.Warnf("project file %s does not exist anymore, skipping", file)
			continue
		}
		validFiles = append(validFiles, file)
	}
	return validFiles
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
	if err := params.composeHelper.Run(ctx, upArgs, nil, writer, writer); err != nil {
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
	buildResult := checkForPersistedFile(configFiles, FeaturesBuildOverrideFilePrefix)
	startResult := checkForPersistedFile(configFiles, FeaturesStartOverrideFilePrefix)

	if !r.shouldRestoreFiles(buildResult.foundLabel, buildResult.fileExists, startResult.fileExists) {
		return false, nil
	}

	if buildResult.fileExists {
		*composeGlobalArgs = append(*composeGlobalArgs, "-f", buildResult.filePath)
	}
	if startResult.fileExists {
		*composeGlobalArgs = append(*composeGlobalArgs, "-f", startResult.filePath)
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
	result, err := r.buildAndExtendDockerCompose(ctx, buildExtendParams{
		parsedConfig:        params.parsedConfig,
		substitutionContext: params.substitutionContext,
		project:             params.project,
		composeHelper:       params.composeHelper,
		composeService:      params.composeService,
		globalArgs:          *params.composeGlobalArgs,
	})
	if err != nil {
		return fmt.Errorf("build and extend docker-compose: %w", err)
	}

	if result.composeFilePath != "" {
		*params.composeGlobalArgs = append(*params.composeGlobalArgs, "-f", result.composeFilePath)
	}

	currentImageName := result.buildImageName
	if currentImageName == "" {
		currentImageName = params.originalImageName
	}

	imageDetails, err := r.inspectImage(ctx, currentImageName)
	if err != nil {
		return fmt.Errorf("inspect image: %w", err)
	}

	imageMetadata, err := r.mergeExtraConfig(result.imageMetadata, params.options)
	if err != nil {
		return fmt.Errorf("merge extra config: %w", err)
	}

	mergedConfig, err := config.MergeConfiguration(params.parsedConfig.Config, imageMetadata.Config)
	if err != nil {
		return fmt.Errorf("merge configuration: %w", err)
	}

	return r.addComposeUpOverride(composeUpOverrideParams{
		params:                 params,
		mergedConfig:           mergedConfig,
		imageDetails:           imageDetails,
		metadataLabel:          result.metadataLabel,
		overrideBuildImageName: result.buildImageName,
	})
}

func (r *runner) mergeExtraConfig(
	imageMetadata *config.ImageMetadataConfig,
	options UpOptions,
) (*config.ImageMetadataConfig, error) {
	if options.ExtraDevContainerPath == "" {
		return imageMetadata, nil
	}

	if imageMetadata == nil {
		imageMetadata = &config.ImageMetadataConfig{}
	}

	extraConfig, err := config.ParseDevContainerJSONFile(options.ExtraDevContainerPath)
	if err != nil {
		return nil, err
	}
	config.AddConfigToImageMetadata(extraConfig, imageMetadata)

	return imageMetadata, nil
}

func (r *runner) addComposeUpOverride(overrideParams composeUpOverrideParams) error {
	additionalLabels := map[string]string{
		metadata.ImageMetadataLabel: overrideParams.metadataLabel,
		config.UserLabel:            overrideParams.imageDetails.Config.User,
	}
	overrideComposeUpFilePath, err := r.extendedDockerComposeUp(composeUpParams{
		parsedConfig:      overrideParams.params.parsedConfig,
		mergedConfig:      overrideParams.mergedConfig,
		composeHelper:     overrideParams.params.composeHelper,
		composeService:    overrideParams.params.composeService,
		originalImageName: overrideParams.params.originalImageName,
		overrideImageName: overrideParams.overrideBuildImageName,
		imageDetails:      overrideParams.imageDetails,
		additionalLabels:  additionalLabels,
	})
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
	// Try to restore persisted files if container exists and we're not recreating
	shouldBuild := true
	if params.container != nil && !params.options.Recreate {
		didRestoreFromPersistedShare, err := r.tryRestorePersistedFiles(params.container, &params.composeGlobalArgs)
		if err != nil {
			return err
		}
		if didRestoreFromPersistedShare {
			// Successfully restored, skip build
			shouldBuild = false
		}
	}

	// Build if we didn't restore from persisted files
	if shouldBuild {
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

	return nil
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
	// Only add --no-recreate if container exists and we're not recreating
	if params.container != nil && !params.options.Recreate {
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

// prepareComposeBuildInfo modifies a compose project's devcontainer Dockerfile
// to ensure it can be extended with features.
// If an Image is specified instead of a Build, the metadata from the Image is used to populate the build info.
func (r *runner) prepareComposeBuildInfo(
	ctx context.Context,
	subCtx *config.SubstitutionContext,
	composeService *composetypes.ServiceConfig,
) buildInfoResult {
	if composeService.Build != nil {
		return r.prepareBuildFromDockerfile(subCtx, composeService)
	}
	imageBuildInfo, err := r.getImageBuildInfoFromImage(ctx, subCtx, composeService.Image)
	return buildInfoResult{imageBuildInfo: imageBuildInfo, err: err}
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

// This extends the build information for docker compose containers.
func (r *runner) buildAndExtendDockerCompose(
	ctx context.Context,
	params buildExtendParams,
) (*buildExtendResult, error) {
	var dockerFilePath, dockerfileContents, dockerComposeFilePath string
	var imageBuildInfo *config.ImageBuildInfo

	buildImageName := r.getBuildImageName(params)

	// Determine base imageName for generated features build
	result := r.prepareComposeBuildInfo(
		ctx, params.substitutionContext, params.composeService)
	if result.err != nil {
		return nil, result.err
	}
	imageBuildInfo = result.imageBuildInfo
	dockerfileContents = result.dockerfileContents

	buildTarget := "dev_container_auto_added_stage_label"
	if result.buildTarget != "" {
		buildTarget = result.buildTarget
	}

	extendImageBuildInfo, err := feature.GetExtendedBuildInfo(
		params.substitutionContext, imageBuildInfo, buildTarget, params.parsedConfig, r.Log, false)
	if err != nil {
		return nil, err
	}

	var extendedDockerfilePath string
	dockerComposeFilePath, extendedDockerfilePath, err = r.handleFeaturesBuild(
		params, extendImageBuildInfo, featuresBuildContext{
			dockerFilePath:     dockerFilePath,
			dockerfileContents: dockerfileContents,
			buildTarget:        buildTarget,
		})
	if err != nil {
		return nil, err
	}
	if extendedDockerfilePath != "" {
		defer func() { _ = os.RemoveAll(filepath.Dir(extendedDockerfilePath)) }()
	}

	buildArgs := r.prepareBuildArgs(params, dockerComposeFilePath, extendImageBuildInfo)

	// build image
	writer := r.Log.Writer(logrus.InfoLevel, false)
	defer func() { _ = writer.Close() }()
	r.Log.Debugf("Run %s %s", params.composeHelper.Command, strings.Join(buildArgs, " "))
	err = params.composeHelper.Run(ctx, buildArgs, nil, writer, writer)
	if err != nil {
		return nil, err
	}

	imageMetadata, err := metadata.GetDevContainerMetadata(
		params.substitutionContext, imageBuildInfo.Metadata, params.parsedConfig, extendImageBuildInfo.Features)
	if err != nil {
		return nil, err
	}

	return &buildExtendResult{
		buildImageName:  buildImageName,
		composeFilePath: dockerComposeFilePath,
		imageMetadata:   imageMetadata,
		metadataLabel:   extendImageBuildInfo.MetadataLabel,
	}, nil
}

func (r *runner) getBuildImageName(params buildExtendParams) string {
	buildImageName := params.composeService.Image
	// If Image is empty then we are building the dev container and use the default name docker-compose uses
	if buildImageName == "" {
		buildImageName = fmt.Sprintf("%s-%s", params.project.Name, params.composeService.Name)
	}
	return buildImageName
}

type featuresBuildContext struct {
	dockerFilePath     string
	dockerfileContents string
	buildTarget        string
}

func (r *runner) handleFeaturesBuild(
	params buildExtendParams,
	extendImageBuildInfo *feature.ExtendedBuildInfo,
	buildCtx featuresBuildContext,
) (string, string, error) {
	if extendImageBuildInfo == nil || extendImageBuildInfo.FeaturesBuildInfo == nil {
		return "", "", nil
	}

	dockerfileContents := buildCtx.dockerfileContents
	// If the dockerfile is empty (because an Image was used) reference that image
	// as the build target after the features / modified contents
	if dockerfileContents == "" {
		dockerfileContents = fmt.Sprintf("FROM %s AS %s\n", params.composeService.Image, buildCtx.buildTarget)
	}

	// Write the final Dockerfile with features
	extendedDockerfilePath, extendedDockerfileContent := r.extendedDockerfile(
		extendImageBuildInfo.FeaturesBuildInfo,
		buildCtx.dockerFilePath,
		dockerfileContents,
	)

	r.Log.Debugf(
		"Creating extended Dockerfile %s with content: \n %s",
		extendedDockerfilePath,
		extendedDockerfileContent,
	)

	err := os.WriteFile(extendedDockerfilePath, []byte(extendedDockerfileContent), 0600)
	if err != nil {
		return "", "", fmt.Errorf("write Dockerfile with features: %w", err)
	}

	// Write the final docker-compose referencing the modified Dockerfile or Image
	composeFilePath, err := r.extendedDockerComposeBuild(
		params.composeService,
		extendedDockerfilePath,
		extendImageBuildInfo.FeaturesBuildInfo,
	)
	return composeFilePath, extendedDockerfilePath, err
}

func (r *runner) prepareBuildArgs(
	params buildExtendParams,
	dockerComposeFilePath string,
	extendImageBuildInfo *feature.ExtendedBuildInfo,
) []string {
	buildArgs := []string{"--project-name", params.project.Name}
	buildArgs = append(buildArgs, params.globalArgs...)
	if dockerComposeFilePath != "" {
		buildArgs = append(buildArgs, "-f", dockerComposeFilePath)
	}
	buildArgs = append(buildArgs, "build")
	if extendImageBuildInfo == nil {
		buildArgs = append(buildArgs, "--pull")
	}

	// Only run the services defined in .devcontainer.json runServices
	if len(params.parsedConfig.Config.RunServices) > 0 {
		buildArgs = append(buildArgs, params.composeService.Name)
		for _, service := range params.parsedConfig.Config.RunServices {
			if service == params.composeService.Name {
				continue
			}
			buildArgs = append(buildArgs, service)
		}
	}

	return buildArgs
}

func (r *runner) extendedDockerfile(
	featureBuildInfo *feature.BuildInfo, dockerfilePath, dockerfileContent string,
) (string, string) {
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

func (r *runner) extendedDockerComposeBuild(
	composeService *composetypes.ServiceConfig, dockerFilePath string, featuresBuildInfo *feature.BuildInfo,
) (string, error) {
	service := r.createBuildService(composeService, dockerFilePath, featuresBuildInfo)
	project := &composetypes.Project{
		Services: map[string]composetypes.ServiceConfig{service.Name: *service},
	}
	return r.writeBuildComposeFile(project)
}

func (r *runner) createBuildService(
	composeService *composetypes.ServiceConfig, dockerFilePath string, featuresBuildInfo *feature.BuildInfo,
) *composetypes.ServiceConfig {
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
	return service
}

func (r *runner) writeBuildComposeFile(project *composetypes.Project) (string, error) {
	dockerComposeFolder := getDockerComposeFolder(r.WorkspaceConfig.Origin)
	err := os.MkdirAll(dockerComposeFolder, 0750)
	if err != nil {
		return "", err
	}

	dockerComposeData, err := yaml.Marshal(project)
	if err != nil {
		return "", err
	}

	dockerComposePath := filepath.Join(
		dockerComposeFolder, fmt.Sprintf("%s-%d.yml", FeaturesBuildOverrideFilePrefix, time.Now().UnixNano()))

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
	params composeUpParams,
) (string, error) {
	dockerComposeUpProject := r.generateDockerComposeUpProject(params)
	dockerComposeData, err := yaml.Marshal(dockerComposeUpProject)
	if err != nil {
		return "", err
	}

	dockerComposeFolder := getDockerComposeFolder(r.WorkspaceConfig.Origin)
	err = os.MkdirAll(dockerComposeFolder, 0750)
	if err != nil {
		return "", err
	}

	dockerComposePath := filepath.Join(
		dockerComposeFolder, fmt.Sprintf("%s-%d.yml", FeaturesStartOverrideFilePrefix, time.Now().UnixNano()))

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
	params composeUpParams,
) *composetypes.Project {
	userEntrypoint, userCommand := r.getUserEntrypointAndCommand(params)
	entrypoint := r.buildEntrypoint(params.mergedConfig, userEntrypoint)
	labels := r.buildLabels(params.additionalLabels)
	overrideService := r.createOverrideService(params, userCommand, entrypoint, labels)
	return r.createProjectWithVolumes(overrideService, params.mergedConfig)
}

func (r *runner) getUserEntrypointAndCommand(params composeUpParams) ([]string, []string) {
	userEntrypoint := params.composeService.Entrypoint
	userCommand := params.composeService.Command
	if params.mergedConfig.OverrideCommand != nil && *params.mergedConfig.OverrideCommand {
		return []string{}, []string{}
	}
	if len(userEntrypoint) == 0 {
		userEntrypoint = params.imageDetails.Config.Entrypoint
	}
	if len(userCommand) == 0 {
		userCommand = params.imageDetails.Config.Cmd
	}
	return userEntrypoint, userCommand
}

func (r *runner) buildEntrypoint(
	mergedConfig *config.MergedDevContainerConfig, userEntrypoint []string,
) composetypes.ShellCommand {
	var script strings.Builder
	script.WriteString("echo Container started\ntrap \"exit 0\" 15\nexec \"$$@\"\n")
	if len(mergedConfig.Entrypoints) > 0 {
		script.WriteString(strings.Join(mergedConfig.Entrypoints, "\n"))
		script.WriteByte('\n')
	}
	script.WriteString(DefaultEntrypoint)

	entrypoint := composetypes.ShellCommand{"/bin/sh", "-c", script.String(), "-"}
	return append(entrypoint, userEntrypoint...)
}

func (r *runner) buildLabels(additionalLabels map[string]string) composetypes.Labels {
	labels := composetypes.Labels{config.DockerIDLabel: r.ID}
	for k, v := range additionalLabels {
		label := regexp.MustCompile(`\$`).ReplaceAllString(v, "$$$$")
		label = regexp.MustCompile(`'`).ReplaceAllString(label, `\'\'`)
		labels.Add(k, label)
	}
	return labels
}

func (r *runner) createOverrideService(
	params composeUpParams, userCommand []string, entrypoint composetypes.ShellCommand, labels composetypes.Labels,
) *composetypes.ServiceConfig {
	overrideService := &composetypes.ServiceConfig{
		Name:        params.composeService.Name,
		Entrypoint:  entrypoint,
		Environment: mappingFromMap(params.mergedConfig.ContainerEnv),
		Init:        params.mergedConfig.Init,
		CapAdd:      params.mergedConfig.CapAdd,
		SecurityOpt: params.mergedConfig.SecurityOpt,
		Labels:      labels,
	}

	if params.originalImageName != params.overrideImageName {
		overrideService.Image = params.overrideImageName
	}

	if !reflect.DeepEqual(userCommand, params.composeService.Command) {
		overrideService.Command = userCommand
	}

	if params.mergedConfig.ContainerUser != "" {
		overrideService.User = params.mergedConfig.ContainerUser
	}

	if params.mergedConfig.Privileged != nil {
		overrideService.Privileged = *params.mergedConfig.Privileged
	}

	gpuSupportEnabled, _ := params.composeHelper.Docker.GPUSupportEnabled()
	r.configureGPUResources(params.parsedConfig, gpuSupportEnabled, overrideService)

	for _, mount := range params.mergedConfig.Mounts {
		overrideService.Volumes = append(overrideService.Volumes, composetypes.ServiceVolumeConfig{
			Type:   mount.Type,
			Source: mount.Source,
			Target: mount.Target,
		})
	}
	return overrideService
}

func (r *runner) createProjectWithVolumes(
	overrideService *composetypes.ServiceConfig, mergedConfig *config.MergedDevContainerConfig,
) *composetypes.Project {
	project := &composetypes.Project{
		Services: map[string]composetypes.ServiceConfig{overrideService.Name: *overrideService},
	}

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
		for _, volumeMount := range volumeMounts {
			project.Volumes[volumeMount.Name] = volumeMount
		}
	}

	return project
}

func (r *runner) configureGPUResources(
	parsedConfig *config.SubstitutedConfig, gpuSupportEnabled bool, overrideService *composetypes.ServiceConfig,
) {
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

type persistedFileResult struct {
	foundLabel bool
	fileExists bool
	filePath   string
}

func checkForPersistedFile(files []string, prefix string) persistedFileResult {
	for _, file := range files {
		if !strings.HasPrefix(file, prefix) {
			continue
		}

		stat, err := os.Stat(file)
		if err == nil && stat.Mode().IsRegular() {
			return persistedFileResult{true, true, file}
		} else if os.IsNotExist(err) {
			return persistedFileResult{true, false, file}
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
