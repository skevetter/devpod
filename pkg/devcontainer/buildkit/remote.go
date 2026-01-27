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
	imageName := repo + "/" + build.GetImageName(opts.LocalWorkspaceFolder, opts.PrebuildHash)
	ref, keychain, err := resolveImageReference(ctx, imageName)
	if err != nil {
		return nil, err
	}

	if buildInfo, found := checkExistingImage(checkExistingImageOptions{
		Ref:               ref,
		TargetArch:        opts.TargetArch,
		Keychain:          keychain,
		ImageName:         imageName,
		PrebuildHash:      opts.PrebuildHash,
		ExtendedBuildInfo: opts.ExtendedBuildInfo,
		Options:           opts.Options,
		Log:               opts.Log,
	}); found {
		return buildInfo, nil
	}

	if err := remote.CheckPushPermission(ref, keychain, http.DefaultTransport); err != nil {
		return nil, fmt.Errorf("pushing %s is not allowed %w", ref, err)
	}

	session, err := setupRegistryAuth(ref, keychain)
	if err != nil {
		return nil, err
	}

	buildOpts, cacheFrom, cacheTo, err := prepareBuildOptions(prepareBuildOptionsParams{
		DockerfilePath:    opts.DockerfilePath,
		DockerfileContent: opts.DockerfileContent,
		ParsedConfig:      opts.ParsedConfig,
		ExtendedBuildInfo: opts.ExtendedBuildInfo,
		ImageName:         imageName,
		Options:           opts.Options,
		PrebuildHash:      opts.PrebuildHash,
	})
	if err != nil {
		return nil, err
	}

	localMounts, err := setupLocalMounts(buildOpts)
	if err != nil {
		return nil, err
	}

	solveOptions, err := createSolveOptions(createSolveOptionsParams{
		BuildOpts:   buildOpts,
		LocalMounts: localMounts,
		Session:     session,
		CacheFrom:   cacheFrom,
		CacheTo:     cacheTo,
		Options:     opts.Options,
		TargetArch:  opts.TargetArch,
	})
	if err != nil {
		return nil, err
	}

	if err := executeBuild(executeBuildParams{
		Ctx:       ctx,
		Client:    c,
		SolveOpts: solveOptions,
		Info:      info,
		BuildOpts: buildOpts,
		Log:       opts.Log,
	}); err != nil {
		return nil, err
	}

	imageDetails, err := getImageDetails(ref, opts.TargetArch, keychain)
	if err != nil {
		return nil, fmt.Errorf("get image details %w", err)
	}

	return &config.BuildInfo{
		ImageDetails:  imageDetails,
		ImageMetadata: opts.ExtendedBuildInfo.MetadataConfig,
		ImageName:     imageName,
		PrebuildHash:  opts.PrebuildHash,
		RegistryCache: opts.Options.RegistryCache,
		Tags:          opts.Options.Tag,
	}, nil
}

func validateRemoteBuildOptions(options provider.BuildOptions) error {
	if options.NoBuild {
		return fmt.Errorf("you cannot build in this mode. Please run 'devpod up' to rebuild the container")
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
	if options.CLIOptions.Platform.Build.Repository == "" && !options.SkipPush {
		return errors.New("remote builds require a registry to be provided")
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

	tmpDir, caPath, keyPath, certPath, err := ensureCertPaths(options.CLIOptions.Platform.Build)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, nil, "", fmt.Errorf("ensure certificates %w", err)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	c, err := client.New(timeoutCtx,
		options.CLIOptions.Platform.Build.RemoteAddress,
		client.WithServerConfig(remoteURL.Hostname(), caPath),
		client.WithCredentials(certPath, keyPath),
	)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, nil, "", fmt.Errorf("get client %w", err)
	}

	info, err := c.Info(timeoutCtx)
	if err != nil {
		_ = c.Close()
		_ = os.RemoveAll(tmpDir)
		return nil, nil, "", fmt.Errorf("get remote builder info %w", err)
	}

	return c, info, tmpDir, nil
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

type checkExistingImageOptions struct {
	Ref               name.Reference
	TargetArch        string
	Keychain          authn.Keychain
	ImageName         string
	PrebuildHash      string
	ExtendedBuildInfo *feature.ExtendedBuildInfo
	Options           provider.BuildOptions
	Log               log.Logger
}

func checkExistingImage(opts checkExistingImageOptions) (*config.BuildInfo, bool) {
	imageDetails, err := getImageDetails(opts.Ref, opts.TargetArch, opts.Keychain)
	if err == nil {
		opts.Log.Infof("skipping build because an existing image was found %s", opts.ImageName)
		return &config.BuildInfo{
			ImageDetails:  imageDetails,
			ImageMetadata: opts.ExtendedBuildInfo.MetadataConfig,
			ImageName:     opts.ImageName,
			PrebuildHash:  opts.PrebuildHash,
			RegistryCache: opts.Options.RegistryCache,
			Tags:          opts.Options.Tag,
		}, true
	}
	return nil, false
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

type prepareBuildOptionsParams struct {
	DockerfilePath    string
	DockerfileContent string
	ParsedConfig      *config.SubstitutedConfig
	ExtendedBuildInfo *feature.ExtendedBuildInfo
	ImageName         string
	Options           provider.BuildOptions
	PrebuildHash      string
}

func prepareBuildOptions(params prepareBuildOptionsParams) (*build.BuildOptions, []client.CacheOptionsEntry, []client.CacheOptionsEntry, error) {
	buildOpts, err := build.NewOptions(params.DockerfilePath, params.DockerfileContent, params.ParsedConfig, params.ExtendedBuildInfo, params.ImageName, params.Options, params.PrebuildHash)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create build options %w", err)
	}

	cacheFrom, err := ParseCacheEntry(buildOpts.CacheFrom)
	if err != nil {
		return nil, nil, nil, err
	}

	cacheTo, err := ParseCacheEntry(buildOpts.CacheTo)
	if err != nil {
		return nil, nil, nil, err
	}

	return buildOpts, cacheFrom, cacheTo, nil
}

func setupLocalMounts(buildOpts *build.BuildOptions) (map[string]fsutil.FS, error) {
	localMounts := map[string]fsutil.FS{}

	dockerfileDir := filepath.Dir(buildOpts.Dockerfile)
	dockerfileMount, err := fsutil.NewFS(dockerfileDir)
	if err != nil {
		return nil, fmt.Errorf("create local dockerfile mount %w", err)
	}
	localMounts["dockerfile"] = dockerfileMount

	contextMount, err := fsutil.NewFS(buildOpts.Context)
	if err != nil {
		return nil, fmt.Errorf("create local context mount %w", err)
	}
	localMounts["context"] = contextMount

	return localMounts, nil
}

type createSolveOptionsParams struct {
	BuildOpts   *build.BuildOptions
	LocalMounts map[string]fsutil.FS
	Session     []session.Attachable
	CacheFrom   []client.CacheOptionsEntry
	CacheTo     []client.CacheOptionsEntry
	Options     provider.BuildOptions
	TargetArch  string
}

func createSolveOptions(params createSolveOptionsParams) (client.SolveOpt, error) {
	solveOpts := client.SolveOpt{
		Frontend: "dockerfile.v0",
		FrontendAttrs: map[string]string{
			"filename": filepath.Base(params.BuildOpts.Dockerfile),
			"context":  params.BuildOpts.Context,
		},
		LocalMounts:  params.LocalMounts,
		Session:      params.Session,
		CacheImports: params.CacheFrom,
		CacheExports: params.CacheTo,
	}

	if params.BuildOpts.Target != "" {
		solveOpts.FrontendAttrs["target"] = params.BuildOpts.Target
	}

	if params.Options.Platform != "" {
		solveOpts.FrontendAttrs["platform"] = params.Options.Platform
	} else if params.TargetArch != "" {
		solveOpts.FrontendAttrs["platform"] = "linux/" + params.TargetArch
	}

	if err := addMultiContexts(&solveOpts, params.BuildOpts); err != nil {
		return client.SolveOpt{}, err
	}

	push := "true"
	if params.Options.SkipPush {
		push = "false"
	}
	solveOpts.Exports = append(solveOpts.Exports, client.ExportEntry{
		Type: client.ExporterImage,
		Attrs: map[string]string{
			string(exptypes.OptKeyName): strings.Join(params.BuildOpts.Images, ","),
			string(exptypes.OptKeyPush): push,
		},
	})

	for k, v := range params.BuildOpts.Labels {
		solveOpts.FrontendAttrs["label:"+k] = v
	}

	for key, value := range params.BuildOpts.BuildArgs {
		solveOpts.FrontendAttrs["build-arg:"+key] = value
	}

	return solveOpts, nil
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
	SolveOpts client.SolveOpt
	Info      *client.Info
	BuildOpts *build.BuildOptions
	Log       log.Logger
}

func executeBuild(params executeBuildParams) error {
	params.Log.Infof("start building %s using platform builder (%s)", strings.Join(params.BuildOpts.Images, ","), params.Info.BuildkitVersion.Version)

	writer := params.Log.Writer(logrus.InfoLevel, false)
	defer func() { _ = writer.Close() }()

	pw, err := NewPrinter(params.Ctx, writer)
	if err != nil {
		return err
	}

	_, err = params.Client.Solve(params.Ctx, nil, params.SolveOpts, pw.Status())
	return err
}

func getImageDetails(ref name.Reference, targetArch string, keychain authn.Keychain) (*config.ImageDetails, error) {
	remoteImage, err := remote.Image(ref,
		remote.WithAuthFromKeychain(keychain),
		remote.WithPlatform(v1.Platform{Architecture: targetArch, OS: "linux"}),
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

func ensureCertPaths(buildOpts *devpod.PlatformBuildOptions) (parentDir string, caPath string, keyPath string, certPath string, err error) {
	parentDir, err = os.MkdirTemp("", "build-certs-*")
	if err != nil {
		return parentDir, caPath, keyPath, caPath, fmt.Errorf("create temp dir %w", err)
	}

	// write CA
	caPath = filepath.Join(parentDir, "ca.pem")
	caBytes, err := base64.StdEncoding.DecodeString(buildOpts.CertCA)
	if err != nil {
		return parentDir, caPath, keyPath, caPath, fmt.Errorf("decode CA %w", err)
	}
	err = os.WriteFile(caPath, caBytes, 0o700)
	if err != nil {
		return parentDir, caPath, keyPath, caPath, fmt.Errorf("write CA file %w", err)
	}

	// write key
	keyPath = filepath.Join(parentDir, "key.pem")
	keyBytes, err := base64.StdEncoding.DecodeString(buildOpts.CertKey)
	if err != nil {
		return parentDir, caPath, keyPath, caPath, fmt.Errorf("decode private key %w", err)
	}
	err = os.WriteFile(keyPath, keyBytes, 0o700)
	if err != nil {
		return parentDir, caPath, keyPath, caPath, fmt.Errorf("write private key file %w", err)
	}

	// write cert
	certPath = filepath.Join(parentDir, "cert.pem")
	certBytes, err := base64.StdEncoding.DecodeString(buildOpts.Cert)
	if err != nil {
		return parentDir, caPath, keyPath, caPath, fmt.Errorf("decode cert %w", err)
	}
	err = os.WriteFile(certPath, certBytes, 0o700)
	if err != nil {
		return parentDir, caPath, keyPath, caPath, fmt.Errorf("write cert file %w", err)
	}

	return parentDir, caPath, keyPath, caPath, nil
}
