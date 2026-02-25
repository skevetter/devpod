package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/moby/patternmatcher"
	"github.com/moby/patternmatcher/ignorefile"
	"github.com/sirupsen/logrus"
	util "github.com/skevetter/devpod/pkg/util/hash"
	"github.com/skevetter/log"
	"github.com/skevetter/log/hash"
)

// PrebuildHashParams contains all parameters needed to calculate a prebuild hash.
type PrebuildHashParams struct {
	Config            *DevContainerConfig
	Platform          string
	Architecture      string
	ContextPath       string
	DockerfilePath    string
	DockerfileContent string
	BuildInfo         *ImageBuildInfo
	Log               log.Logger
}

// CalculatePrebuildHash computes a deterministic hash for prebuild caching.
// The hash includes: architecture, normalized config, dockerfile content, and context files.
// The hash format is "devpod-" followed by 32 hex characters.
func CalculatePrebuildHash(params PrebuildHashParams) (string, error) {
	arch := normalizeArchitecture(params.Platform, params.Architecture)

	configJSON, err := normalizeConfigForHash(params.Config)
	if err != nil {
		return "", fmt.Errorf("failed to normalize config: %w", err)
	}

	excludes, err := readDockerignore(params.ContextPath, params.DockerfilePath)
	if err != nil {
		params.Log.Debugf("failed to read .dockerignore: %v", err)
		return "", fmt.Errorf("failed to read dockerignore: %w", err)
	}
	excludes = append(excludes, DevPodContextFeatureFolder+"/")

	var includes []string
	if params.BuildInfo != nil && params.BuildInfo.Dockerfile != nil {
		includes = params.BuildInfo.Dockerfile.BuildContextFiles()
	}

	contextHash, err := util.DirectoryHash(params.ContextPath, excludes, includes)
	if err != nil {
		params.Log.Debugf("failed to compute context hash for %s: %v", params.ContextPath, err)
		return "", fmt.Errorf("failed to compute context hash: %w", err)
	}

	combined := arch + string(configJSON) + params.DockerfileContent + contextHash
	finalHash := "devpod-" + hash.String(combined)[:32]

	params.Log.WithFields(logrus.Fields{
		"architecture": arch,
		"contextHash":  contextHash,
		"finalHash":    finalHash,
		"excludeCount": len(excludes),
		"includeCount": len(includes),
	}).Debug("prebuild hash calculated")

	return finalHash, nil
}

// normalizeArchitecture extracts architecture from platform string.
// Platform format: "linux/amd64" -> architecture: "amd64".
func normalizeArchitecture(platform, architecture string) string {
	if platform != "" {
		parts := strings.Split(platform, "/")
		if len(parts) >= 2 && parts[0] == "linux" {
			return parts[1]
		}
	}
	return architecture
}

// normalizeConfigForHash creates a config with only build-relevant fields.
// This ensures the hash only changes when build-affecting fields change.
func normalizeConfigForHash(config *DevContainerConfig) ([]byte, error) {
	if config == nil {
		return nil, fmt.Errorf("config is nil")
	}
	normalized := CloneDevContainerConfig(config)

	// Clear non-build fields
	normalized.Origin = ""
	normalized.DevContainerActions = DevContainerActions{}
	normalized.NonComposeBase = NonComposeBase{}
	normalized.DevContainerConfigBase = DevContainerConfigBase{
		Name:                        normalized.Name,
		Features:                    normalized.Features,
		OverrideFeatureInstallOrder: normalized.OverrideFeatureInstallOrder,
	}
	normalized.ImageContainer = ImageContainer{
		Image: normalized.Image,
	}
	normalized.ComposeContainer = ComposeContainer{}
	normalized.DockerfileContainer = DockerfileContainer{
		Dockerfile: normalized.Dockerfile,
		Context:    normalized.Context,
		Build:      normalized.Build,
	}

	return json.Marshal(normalized)
}

// readDockerignore reads the .dockerignore file in the context directory and
// returns the list of paths to exclude.
func readDockerignore(contextDir string, dockerfile string) ([]string, error) {
	var (
		f        *os.File
		err      error
		excludes = []string{}
	)

	dockerignorefilepath := dockerfile + ".dockerignore"
	if filepath.IsAbs(dockerignorefilepath) {
		f, err = os.Open(dockerignorefilepath)
	} else {
		f, err = os.Open(filepath.Join(contextDir, dockerignorefilepath))
	}
	if os.IsNotExist(err) {
		dockerignorefilepath = ".dockerignore"
		f, err = os.Open(filepath.Join(contextDir, dockerignorefilepath))
		if os.IsNotExist(err) {
			return ensureDockerIgnoreAndDockerFile(excludes, dockerfile, dockerignorefilepath), nil
		} else if err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}

	defer func() { _ = f.Close() }()

	excludes, err = ignorefile.ReadAll(f)
	if err != nil {
		return nil, err
	}
	return ensureDockerIgnoreAndDockerFile(excludes, dockerfile, dockerignorefilepath), nil
}

func ensureDockerIgnoreAndDockerFile(excludes []string, dockerfile, dockerignorefilepath string) []string {
	_, dockerignorefile := filepath.Split(dockerignorefilepath)
	if keep, _ := patternmatcher.MatchesOrParentMatches(dockerignorefile, excludes); keep {
		excludes = append(excludes, "!"+dockerignorefile)
	}

	if keep, _ := patternmatcher.MatchesOrParentMatches(dockerfile, excludes); keep {
		excludes = append(excludes, "!"+dockerfile)
	}

	return excludes
}
