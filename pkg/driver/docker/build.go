package docker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/pkg/devcontainer/build"
	"github.com/skevetter/devpod/pkg/devcontainer/buildkit"
	"github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/skevetter/devpod/pkg/devcontainer/feature"
	"github.com/skevetter/devpod/pkg/docker"
	"github.com/skevetter/devpod/pkg/driver"
	"github.com/skevetter/devpod/pkg/provider"
)

func (d *dockerDriver) BuildDevContainer(
	ctx context.Context,
	req driver.BuildRequest,
) (*config.BuildInfo, error) {
	imageName := build.GetImageName(req.LocalWorkspaceFolder, req.PrebuildHash)
	orchestrator := &buildOrchestrator{
		driver:   d,
		resolver: &imageResolver{driver: d},
	}

	if buildInfo, found := orchestrator.resolver.tryResolve(ctx, resolveRequest{
		imageName:         imageName,
		options:           req.Options,
		extendedBuildInfo: req.ExtendedBuildInfo,
		prebuildHash:      req.PrebuildHash,
	}); found {
		return buildInfo, nil
	}

	if req.Options.NoBuild {
		return nil, fmt.Errorf("you cannot build in this mode. Please run 'devpod up' to rebuild the container")
	}

	buildOptions, err := d.prepareBuildOptions(req, imageName)
	if err != nil {
		return nil, err
	}

	if err := d.executeBuild(ctx, orchestrator, req, buildOptions); err != nil {
		return nil, err
	}

	return d.createBuildInfo(ctx, imageName, req, buildOptions)
}

// buildStrategy defines the interface for different build implementations.
type buildStrategy interface {
	build(ctx context.Context, writer io.Writer, platform string, options *build.BuildOptions) error
	isAvailable(ctx context.Context) bool
	name() string
}

// buildxStrategy implements modern Docker build with BuildKit.
type buildxStrategy struct {
	driver *dockerDriver
}

func (s *buildxStrategy) build(
	ctx context.Context,
	writer io.Writer,
	platform string,
	options *build.BuildOptions,
) error {
	args := buildDockerArgs(options, platform)

	s.driver.Log.Debugf("running docker build with args: %s", strings.Join(args, " "))
	stderrBuf := &bytes.Buffer{}
	multiWriter := io.MultiWriter(writer, stderrBuf)
	if err := s.driver.Docker.Run(ctx, args, nil, writer, multiWriter); err != nil {
		if stderrBuf.Len() > 0 {
			return fmt.Errorf("failed to build image: %w: %s", err, strings.TrimSpace(stderrBuf.String()))
		}
		return fmt.Errorf("failed to build image: %w", err)
	}
	return nil
}

func buildDockerArgs(options *build.BuildOptions, platform string) []string {
	args := []string{"build", "-f", options.Dockerfile}
	args = appendBuildFlags(args, options.Load, options.Push)
	args = appendImageTags(args, options.Images)
	args = appendBuildArgsAndContexts(args, options.BuildArgs, options.Contexts)
	args = appendTargetAndPlatform(args, options.Target, platform)
	args = appendCacheOptions(args, options.CacheFrom, options.CacheTo)
	args = append(args, options.CliOpts...)
	args = append(args, options.Context)
	return args
}

func appendBuildFlags(args []string, load, push bool) []string {
	if load {
		args = append(args, "--load")
	}
	if push {
		args = append(args, "--push")
	}
	return args
}

func appendImageTags(args []string, images []string) []string {
	for _, image := range images {
		args = append(args, "-t", image)
	}
	return args
}

func appendBuildArgsAndContexts(args []string, buildArgs, contexts map[string]string) []string {
	for k, v := range buildArgs {
		args = append(args, "--build-arg", k+"="+v)
	}
	for k, v := range contexts {
		args = append(args, "--build-context", k+"="+v)
	}
	return args
}

func appendTargetAndPlatform(args []string, target, platform string) []string {
	if target != "" {
		args = append(args, "--target", target)
	}
	if platform != "" {
		args = append(args, "--platform", platform)
	}
	return args
}

func appendCacheOptions(args []string, cacheFrom, cacheTo []string) []string {
	for _, cache := range cacheFrom {
		args = append(args, "--cache-from", cache)
	}
	for _, cache := range cacheTo {
		args = append(args, "--cache-to", cache)
	}
	return args
}

func (s *buildxStrategy) isAvailable(ctx context.Context) bool {
	// Modern Docker (23.0+) has BuildKit integrated by default
	return true
}

func (s *buildxStrategy) name() string {
	return "docker build"
}

// buildkitStrategy implements buildkit-based builds.
type buildkitStrategy struct {
	driver *dockerDriver
}

func (s *buildkitStrategy) build(
	ctx context.Context,
	writer io.Writer,
	platform string,
	options *build.BuildOptions,
) error {
	dockerClient, err := docker.NewClient(ctx, s.driver.Log)
	if err != nil {
		return fmt.Errorf("create docker client: %w", err)
	}
	defer func() { _ = dockerClient.Close() }()

	buildKitClient, err := buildkit.NewDockerClient(ctx, dockerClient)
	if err != nil {
		return fmt.Errorf("create buildkit client: %w", err)
	}
	defer func() { _ = buildKitClient.Close() }()

	if err := buildkit.Build(ctx, buildKitClient, writer, platform, options, s.driver.Log); err != nil {
		return fmt.Errorf("build: %w", err)
	}
	return nil
}

func (s *buildkitStrategy) isAvailable(ctx context.Context) bool {
	return true
}

func (s *buildkitStrategy) name() string {
	return "internal buildkit"
}

// imageResolver attempts to resolve an existing image.
type imageResolver struct {
	driver *dockerDriver
}

type resolveRequest struct {
	imageName         string
	options           provider.BuildOptions
	extendedBuildInfo *feature.ExtendedBuildInfo
	prebuildHash      string
}

func (r *imageResolver) tryResolve(ctx context.Context, req resolveRequest) (*config.BuildInfo, bool) {
	if req.options.Repository != "" || req.options.ForceBuild {
		return nil, false
	}

	imageDetails, err := r.driver.Docker.InspectImage(ctx, req.imageName, false)
	if err != nil {
		r.driver.Log.Debugf("error trying to find local image %s: %v", req.imageName, err)
		return nil, false
	}

	if imageDetails == nil {
		return nil, false
	}

	r.driver.Log.Infof("found existing local image %s", req.imageName)
	return &config.BuildInfo{
		ImageDetails:  imageDetails,
		ImageMetadata: req.extendedBuildInfo.MetadataConfig,
		ImageName:     req.imageName,
		PrebuildHash:  req.prebuildHash,
		RegistryCache: req.options.RegistryCache,
		Tags:          req.options.Tag,
	}, true
}

// buildOrchestrator coordinates the build process.
type buildOrchestrator struct {
	driver   *dockerDriver
	resolver *imageResolver
}

func (o *buildOrchestrator) selectStrategy(
	ctx context.Context,
	options provider.BuildOptions,
) (buildStrategy, error) {
	buildxStrat := &buildxStrategy{driver: o.driver}
	buildkitStrat := &buildkitStrategy{driver: o.driver}

	if o.shouldUseBuildx(ctx, options, buildxStrat) {
		return buildxStrat, nil
	}

	if o.driver.Docker.Builder == docker.DockerBuilderBuildX && !buildxStrat.isAvailable(ctx) {
		return nil, fmt.Errorf("buildx is not available on your host. Use buildkit builder")
	}

	if o.driver.Docker.Builder == docker.DockerBuilderDefault {
		return nil, fmt.Errorf("invalid docker builder: %s", o.driver.Docker.Builder)
	}

	return buildkitStrat, nil
}

func (o *buildOrchestrator) shouldUseBuildx(
	ctx context.Context,
	options provider.BuildOptions,
	buildxStrat *buildxStrategy,
) bool {
	builder := o.driver.Docker.Builder
	return (builder == docker.DockerBuilderDefault || builder == docker.DockerBuilderBuildX) &&
		buildxStrat.isAvailable(ctx) && !options.ForceInternalBuildKit
}

func (d *dockerDriver) prepareBuildOptions(
	req driver.BuildRequest,
	imageName string,
) (*build.BuildOptions, error) {
	buildOptions, err := build.NewOptions(build.NewOptionsParams{
		DockerfilePath:    req.DockerfilePath,
		DockerfileContent: req.DockerfileContent,
		ParsedConfig:      req.ParsedConfig,
		ExtendedBuildInfo: req.ExtendedBuildInfo,
		ImageName:         imageName,
		Options:           req.Options,
		PrebuildHash:      req.PrebuildHash,
	})
	if err != nil {
		return nil, err
	}

	d.Log.Debugf("prepared build options: %+v", buildOptions)
	return buildOptions, nil
}

func (d *dockerDriver) executeBuild(
	ctx context.Context,
	orchestrator *buildOrchestrator,
	req driver.BuildRequest,
	buildOptions *build.BuildOptions,
) error {
	strategy, err := orchestrator.selectStrategy(ctx, req.Options)
	if err != nil {
		return err
	}

	d.Log.Infof("build with %s", strategy.name())
	writer := d.Log.Writer(logrus.InfoLevel, false)
	defer func() { _ = writer.Close() }()

	if err := strategy.build(ctx, writer, req.Options.Platform, buildOptions); err != nil {
		return fmt.Errorf("%s build: %w", strategy.name(), err)
	}
	return nil
}

func (d *dockerDriver) createBuildInfo(
	ctx context.Context,
	imageName string,
	req driver.BuildRequest,
	buildOptions *build.BuildOptions,
) (*config.BuildInfo, error) {
	// When pushing, image may not be available locally
	var imageDetails *config.ImageDetails
	if !buildOptions.Push {
		var err error
		imageDetails, err = d.Docker.InspectImage(ctx, imageName, false)
		if err != nil {
			return nil, fmt.Errorf("get image details: %w", err)
		}
	}

	return &config.BuildInfo{
		ImageDetails:  imageDetails,
		ImageMetadata: req.ExtendedBuildInfo.MetadataConfig,
		ImageName:     imageName,
		PrebuildHash:  req.PrebuildHash,
		RegistryCache: req.Options.RegistryCache,
		Tags:          req.Options.Tag,
	}, nil
}
