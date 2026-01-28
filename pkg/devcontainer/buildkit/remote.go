package buildkit

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/cli/cli/config/types"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/loft-sh/api/v4/pkg/devpod"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/exporter/containerimage/exptypes"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/auth/authprovider"
	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/pkg/devcontainer/build"
	"github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/skevetter/devpod/pkg/devcontainer/feature"
	"github.com/skevetter/devpod/pkg/image"
	"github.com/skevetter/devpod/pkg/provider"
	"github.com/skevetter/log"
	"github.com/tonistiigi/fsutil"
)

type BuildRemoteOptions struct {
	PrebuildHash         string
	ParsedConfig         *config.SubstitutedConfig
	ExtendedBuildInfo    *feature.ExtendedBuildInfo
	DockerfilePath       string
	DockerfileContent    string
	LocalWorkspaceFolder string
	Options              provider.BuildOptions
	TargetArch           string
	Log                  log.Logger
}

func BuildRemote(ctx context.Context, opts BuildRemoteOptions) (*config.BuildInfo, error) {
	if err := validateRemoteBuildOptions(opts.Options); err != nil {
		return nil, err
	}

	c, info, tmpDir, err := setupBuildKitClient(ctx, opts.Options)
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()
	defer func() { _ = c.Close() }()

	repo := strings.TrimSuffix(opts.Options.CLIOptions.Platform.Build.Repository, "/")
	imageName := filepath.Join(repo, build.GetImageName(opts.LocalWorkspaceFolder, opts.PrebuildHash))
	ref, keychain, err := resolveImageReference(ctx, imageName)
	if err != nil {
		return nil, err
	}

	if buildInfo := checkExistingImage(checkExistingImageParams{
		Ctx:        ctx,
		Ref:        ref,
		TargetArch: opts.TargetArch,
		Keychain:   keychain,
		ImageName:  imageName,
		Opts:       opts,
	}); buildInfo != nil {
		return buildInfo, nil
	}

	if err := remote.CheckPushPermission(ref, keychain, http.DefaultTransport); err != nil {
		return nil, fmt.Errorf("pushing %s is not allowed %w", ref, err)
	}

	solveOpts, err := prepareSolveOptions(ref, keychain, imageName, opts)
	if err != nil {
		return nil, err
	}

	if err := executeBuild(executeBuildParams{
		Ctx:       ctx,
		Client:    c,
		Info:      info,
		SolveOpts: solveOpts,
		Logger:    opts.Log,
	}); err != nil {
		return nil, err
	}

	imageDetails, err := getImageDetails(ctx, ref, opts.TargetArch, keychain)
	if err != nil {
		return nil, fmt.Errorf("get image details %w", err)
	}

	var imageMetadata *config.ImageMetadataConfig
	if opts.ExtendedBuildInfo != nil {
		imageMetadata = opts.ExtendedBuildInfo.MetadataConfig
	}

	return &config.BuildInfo{
		ImageDetails:  imageDetails,
		ImageMetadata: imageMetadata,
		ImageName:     imageName,
		PrebuildHash:  opts.PrebuildHash,
		RegistryCache: opts.Options.RegistryCache,
		Tags:          opts.Options.Tag,
	}, nil
}

func validateRemoteBuildOptions(options provider.BuildOptions) error {
	if options.NoBuild {
		return fmt.Errorf("cannot build in this mode, rebuild the container with up command")
	}
	if !options.CLIOptions.Platform.Enabled {
		return errors.New("remote builds are only supported in DevPod Pro")
	}
	if options.CLIOptions.Platform.Build == nil {
		return errors.New("build options are required for remote builds")
	}
	if options.CLIOptions.Platform.Build.RemoteAddress == "" {
		return errors.New("builder address is required to build image remotely")
	}
	if options.SkipPush {
		return errors.New("remote builds require pushing to a registry")
	}
	if options.CLIOptions.Platform.Build.Repository == "" {
		return errors.New("remote builds require a registry to be provided")
	}
	return nil
}

func setupBuildKitClient(ctx context.Context, options provider.BuildOptions) (*client.Client, *client.Info, string, error) {
	remoteURL, err := url.Parse(options.CLIOptions.Platform.Build.RemoteAddress)
	if err != nil {
		return nil, nil, "", err
	}

	certs, err := ensureCertPaths(options.CLIOptions.Platform.Build)
	if err != nil {
		return nil, nil, "", fmt.Errorf("ensure certificates %w", err)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	c, err := client.New(timeoutCtx,
		options.CLIOptions.Platform.Build.RemoteAddress,
		client.WithServerConfig(remoteURL.Hostname(), certs.CAPath),
		client.WithCredentials(certs.CertPath, certs.KeyPath),
	)
	if err != nil {
		_ = os.RemoveAll(certs.ParentDir)
		return nil, nil, "", fmt.Errorf("get client %w", err)
	}

	info, err := c.Info(timeoutCtx)
	if err != nil {
		_ = c.Close()
		_ = os.RemoveAll(certs.ParentDir)
		return nil, nil, "", fmt.Errorf("get remote builder info %w", err)
	}

	return c, info, certs.ParentDir, nil
}

func resolveImageReference(ctx context.Context, imageName string) (name.Reference, authn.Keychain, error) {
	ref, err := name.ParseReference(imageName)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to resolve image %s %w", imageName, err)
	}

	keychain, err := image.GetKeychain(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("get docker auth keychain %w", err)
	}

	return ref, keychain, nil
}

type checkExistingImageParams struct {
	Ctx        context.Context
	Ref        name.Reference
	TargetArch string
	Keychain   authn.Keychain
	ImageName  string
	Opts       BuildRemoteOptions
}

func checkExistingImage(params checkExistingImageParams) *config.BuildInfo {
	imageDetails, err := getImageDetails(params.Ctx, params.Ref, params.TargetArch, params.Keychain)
	if err != nil {
		params.Opts.Log.Debugf("image check failed, continuing with build: %v", err)
		return nil
	}

	params.Opts.Log.Infof("skipping build because an existing image was found %s", params.ImageName)

	var imageMetadata *config.ImageMetadataConfig
	if params.Opts.ExtendedBuildInfo != nil {
		imageMetadata = params.Opts.ExtendedBuildInfo.MetadataConfig
	}

	return &config.BuildInfo{
		ImageDetails:  imageDetails,
		ImageMetadata: imageMetadata,
		ImageName:     params.ImageName,
		PrebuildHash:  params.Opts.PrebuildHash,
		RegistryCache: params.Opts.Options.RegistryCache,
		Tags:          params.Opts.Options.Tag,
	}
}

func setupRegistryAuth(ref name.Reference, keychain authn.Keychain) ([]session.Attachable, error) {
	auth, err := keychain.Resolve(ref.Context())
	if err != nil {
		return nil, fmt.Errorf("get authentication for %s %w", ref.Context().String(), err)
	}

	authConfig, err := auth.Authorization()
	if err != nil {
		return nil, fmt.Errorf("get auth config for %s %w", ref.Context().String(), err)
	}

	registry := ref.Context().RegistryStr()
	return []session.Attachable{
		authprovider.NewDockerAuthProvider(authprovider.DockerAuthProviderConfig{
			AuthConfigProvider: func(ctx context.Context, host string, scope []string, cacheCheck authprovider.ExpireCachedAuthCheck) (types.AuthConfig, error) {
				if host == registry {
					return types.AuthConfig{
						Username:      authConfig.Username,
						Auth:          authConfig.Auth,
						Password:      authConfig.Password,
						IdentityToken: authConfig.IdentityToken,
						RegistryToken: authConfig.RegistryToken,
					}, nil
				}
				return types.AuthConfig{}, nil
			},
		}),
	}, nil
}

func prepareSolveOptions(ref name.Reference, keychain authn.Keychain, imageName string, opts BuildRemoteOptions) (client.SolveOpt, error) {
	session, err := setupRegistryAuth(ref, keychain)
	if err != nil {
		return client.SolveOpt{}, err
	}

	buildOpts, err := build.NewOptions(build.NewOptionsParams{
		DockerfilePath:    opts.DockerfilePath,
		DockerfileContent: opts.DockerfileContent,
		ParsedConfig:      opts.ParsedConfig,
		ExtendedBuildInfo: opts.ExtendedBuildInfo,
		ImageName:         imageName,
		Options:           opts.Options,
		PrebuildHash:      opts.PrebuildHash,
	})
	if err != nil {
		return client.SolveOpt{}, fmt.Errorf("create build options %w", err)
	}

	cacheFrom, cacheTo, err := setupCache(buildOpts)
	if err != nil {
		return client.SolveOpt{}, err
	}

	localMounts, err := setupLocalMounts(buildOpts)
	if err != nil {
		return client.SolveOpt{}, err
	}

	solveOpts := client.SolveOpt{
		Frontend: "dockerfile.v0",
		FrontendAttrs: map[string]string{
			"filename": filepath.Base(buildOpts.Dockerfile),
			"context":  buildOpts.Context,
		},
		LocalMounts:  localMounts,
		Session:      session,
		CacheImports: cacheFrom,
		CacheExports: cacheTo,
	}

	configurePlatform(&solveOpts, buildOpts, opts)

	if err := addMultiContexts(&solveOpts, buildOpts); err != nil {
		return client.SolveOpt{}, err
	}

	addExports(&solveOpts, buildOpts, opts.Options.SkipPush)
	addFrontendAttrs(&solveOpts, buildOpts)

	return solveOpts, nil
}

func setupCache(buildOpts *build.BuildOptions) ([]client.CacheOptionsEntry, []client.CacheOptionsEntry, error) {
	cacheFrom, err := ParseCacheEntry(buildOpts.CacheFrom)
	if err != nil {
		return nil, nil, err
	}

	cacheTo, err := ParseCacheEntry(buildOpts.CacheTo)
	if err != nil {
		return nil, nil, err
	}

	return cacheFrom, cacheTo, nil
}

func setupLocalMounts(buildOpts *build.BuildOptions) (map[string]fsutil.FS, error) {
	dockerfileMount, err := fsutil.NewFS(filepath.Dir(buildOpts.Dockerfile))
	if err != nil {
		return nil, fmt.Errorf("create local dockerfile mount %w", err)
	}

	contextMount, err := fsutil.NewFS(buildOpts.Context)
	if err != nil {
		return nil, fmt.Errorf("create local context mount %w", err)
	}

	return map[string]fsutil.FS{
		"dockerfile": dockerfileMount,
		"context":    contextMount,
	}, nil
}

func configurePlatform(solveOpts *client.SolveOpt, buildOpts *build.BuildOptions, opts BuildRemoteOptions) {
	if buildOpts.Target != "" {
		solveOpts.FrontendAttrs["target"] = buildOpts.Target
	}

	if opts.Options.Platform != "" {
		solveOpts.FrontendAttrs["platform"] = opts.Options.Platform
	} else if opts.TargetArch != "" {
		solveOpts.FrontendAttrs["platform"] = "linux/" + opts.TargetArch
	}
}

func addExports(solveOpts *client.SolveOpt, buildOpts *build.BuildOptions, skipPush bool) {
	push := "true"
	if skipPush {
		push = "false"
	}
	solveOpts.Exports = append(solveOpts.Exports, client.ExportEntry{
		Type: client.ExporterImage,
		Attrs: map[string]string{
			string(exptypes.OptKeyName): strings.Join(buildOpts.Images, ","),
			string(exptypes.OptKeyPush): push,
		},
	})
}

func addFrontendAttrs(solveOpts *client.SolveOpt, buildOpts *build.BuildOptions) {
	for k, v := range buildOpts.Labels {
		solveOpts.FrontendAttrs["label:"+k] = v
	}

	for key, value := range buildOpts.BuildArgs {
		solveOpts.FrontendAttrs["build-arg:"+key] = value
	}
}

func addMultiContexts(solveOpts *client.SolveOpt, buildOpts *build.BuildOptions) error {
	for k, v := range buildOpts.Contexts {
		st, err := os.Stat(v)
		if err != nil {
			return fmt.Errorf("get build context %v %w", k, err)
		}
		if !st.IsDir() {
			return fmt.Errorf("build context '%s' is not a directory", v)
		}

		localName := k
		if k == "context" || k == "dockerfile" {
			localName = "_" + k
		}

		solveOpts.LocalMounts[localName], err = fsutil.NewFS(v)
		if err != nil {
			return fmt.Errorf("create local mount for %s at %s %w", localName, v, err)
		}

		solveOpts.FrontendAttrs["context:"+k] = "local:" + localName
	}
	return nil
}

type executeBuildParams struct {
	Ctx       context.Context
	Client    *client.Client
	Info      *client.Info
	SolveOpts client.SolveOpt
	Logger    log.Logger
}

func executeBuild(params executeBuildParams) error {
	params.Logger.Infof("start building %s using platform builder (%s)", params.SolveOpts.Exports[0].Attrs[string(exptypes.OptKeyName)], params.Info.BuildkitVersion.Version)

	writer := params.Logger.Writer(logrus.InfoLevel, false)
	defer func() { _ = writer.Close() }()

	pw, err := NewPrinter(params.Ctx, writer)
	if err != nil {
		return err
	}

	_, err = params.Client.Solve(params.Ctx, nil, params.SolveOpts, pw.Status())
	return err
}

func getImageDetails(ctx context.Context, ref name.Reference, targetArch string, keychain authn.Keychain) (*config.ImageDetails, error) {
	remoteImage, err := remote.Image(ref,
		remote.WithAuthFromKeychain(keychain),
		remote.WithPlatform(v1.Platform{Architecture: targetArch, OS: "linux"}),
		remote.WithContext(ctx),
	)
	if err != nil {
		return nil, err
	}
	imageConfig, err := remoteImage.ConfigFile()
	if err != nil {
		return nil, fmt.Errorf("get image config file %w", err)
	}

	imageDetails := &config.ImageDetails{
		ID: ref.Name(),
		Config: config.ImageDetailsConfig{
			User:       imageConfig.Config.User,
			Env:        imageConfig.Config.Env,
			Labels:     imageConfig.Config.Labels,
			Entrypoint: imageConfig.Config.Entrypoint,
			Cmd:        imageConfig.Config.Cmd,
		},
	}

	return imageDetails, nil
}

type certPaths struct {
	ParentDir string
	CAPath    string
	KeyPath   string
	CertPath  string
}

func ensureCertPaths(buildOpts *devpod.PlatformBuildOptions) (*certPaths, error) {
	parentDir, err := os.MkdirTemp("", "build-certs-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir %w", err)
	}

	caPath, err := writeCertFile(parentDir, "ca.pem", buildOpts.CertCA, "CA")
	if err != nil {
		return nil, err
	}

	keyPath, err := writeCertFile(parentDir, "key.pem", buildOpts.CertKey, "private key")
	if err != nil {
		return nil, err
	}

	certPath, err := writeCertFile(parentDir, "cert.pem", buildOpts.Cert, "cert")
	if err != nil {
		return nil, err
	}

	return &certPaths{
		ParentDir: parentDir,
		CAPath:    caPath,
		KeyPath:   keyPath,
		CertPath:  certPath,
	}, nil
}

func writeCertFile(parentDir, filename, base64Content, certType string) (string, error) {
	filePath := filepath.Join(parentDir, filename)

	certBytes, err := base64.StdEncoding.DecodeString(base64Content)
	if err != nil {
		return filePath, fmt.Errorf("decode %s %w", certType, err)
	}

	err = os.WriteFile(filePath, certBytes, 0o600)
	if err != nil {
		return filePath, fmt.Errorf("write %s file %w", certType, err)
	}

	return filePath, nil
}
