package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/moby/patternmatcher"
	"github.com/moby/patternmatcher/ignorefile"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	util "github.com/skevetter/devpod/pkg/util/hash"

	"github.com/skevetter/log"
	"github.com/skevetter/log/hash"
)

func CalculatePrebuildHash(
	originalConfig *DevContainerConfig,
	platform, architecture, contextPath, dockerfilePath, dockerfileContent string,
	buildInfo *ImageBuildInfo,
	log log.Logger) (string, error) {

	log.WithFields(logrus.Fields{
		"platform":       platform,
		"architecture":   architecture,
		"contextPath":    contextPath,
		"dockerfilePath": dockerfilePath,
	}).Debug("starting prebuild hash calculation")

	parsedConfig := CloneDevContainerConfig(originalConfig)

	if platform != "" {
		splitted := strings.Split(platform, "/")
		if len(splitted) == 2 && splitted[0] == "linux" {
			architecture = splitted[1]
		}
	}

	// delete all options that are not relevant for the build
	parsedConfig.Origin = ""
	parsedConfig.DevContainerActions = DevContainerActions{}
	parsedConfig.NonComposeBase = NonComposeBase{}
	parsedConfig.DevContainerConfigBase = DevContainerConfigBase{
		Name:                        parsedConfig.Name,
		Features:                    parsedConfig.Features,
		OverrideFeatureInstallOrder: parsedConfig.OverrideFeatureInstallOrder,
	}
	parsedConfig.ImageContainer = ImageContainer{
		Image: parsedConfig.Image,
	}
	parsedConfig.ComposeContainer = ComposeContainer{}
	parsedConfig.DockerfileContainer = DockerfileContainer{
		Dockerfile: parsedConfig.Dockerfile,
		Context:    parsedConfig.Context,
		Build:      parsedConfig.Build,
	}

	// marshal the config
	configStr, err := json.Marshal(parsedConfig)
	if err != nil {
		return "", err
	}

	// find out excludes from dockerignore
	excludes, err := readDockerignore(contextPath, dockerfilePath)
	if err != nil {
		log.WithFields(logrus.Fields{"error": err}).Error("failed to read .dockerignore")
		return "", errors.Errorf("Error reading .dockerignore: %v", err)
	}
	excludes = append(excludes, DevPodContextFeatureFolder+"/")

	log.WithFields(logrus.Fields{
		"excludes":    excludes,
		"contextPath": contextPath,
	}).Debug("docker ignore patterns loaded")

	// find exact files to hash
	// todo pass down target or search all
	// todo update DirectoryHash function
	var includes []string
	if buildInfo.Dockerfile != nil {
		includes = buildInfo.Dockerfile.BuildContextFiles()
	}
	log.WithFields(logrus.Fields{
		"files": includes,
	}).Debug("build context files to use for hash")

	// get hash of the context directory
	contextHash, err := util.DirectoryHash(contextPath, excludes, includes)
	if err != nil {
		log.WithFields(logrus.Fields{"error": err, "contextPath": contextPath}).Error("failed to calculate context hash")
		return "", err
	}

	finalHash := "devpod-" + hash.String(architecture + string(configStr) + dockerfileContent + contextHash)[:32]

	log.WithFields(logrus.Fields{
		"arch":              architecture,
		"config":            string(configStr),
		"dockerfileContent": dockerfileContent,
		"contextHash":       contextHash,
		"finalHash":         finalHash,
	}).Debug("prebuild hash components")

	return finalHash, nil
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
