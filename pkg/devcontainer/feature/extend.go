package feature

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/skevetter/devpod/pkg/copy"
	"github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/skevetter/devpod/pkg/devcontainer/graph"
	"github.com/skevetter/devpod/pkg/devcontainer/metadata"
	"github.com/skevetter/log"
)

var featureSafeIDRegex1 = regexp.MustCompile(`[^\w_]`)
var featureSafeIDRegex2 = regexp.MustCompile(`^[\d_]+`)

const FEATURE_BASE_DOCKERFILE = `
FROM $_DEV_CONTAINERS_BASE_IMAGE AS dev_containers_target_stage

USER root

COPY ./` + config.DevPodContextFeatureFolder + `/ /tmp/build-features/
RUN chmod -R 0755 /tmp/build-features && ls /tmp/build-features

#{featureLayer}

ARG _DEV_CONTAINERS_IMAGE_USER=root
USER $_DEV_CONTAINERS_IMAGE_USER
`

type ExtendedBuildInfo struct {
	Features          []*config.FeatureSet
	FeaturesBuildInfo *BuildInfo

	MetadataConfig *config.ImageMetadataConfig
	MetadataLabel  string
}

type BuildInfo struct {
	FeaturesFolder          string
	DockerfileContent       string
	OverrideTarget          string
	DockerfilePrefixContent string
	BuildArgs               map[string]string
}

func GetExtendedBuildInfo(ctx *config.SubstitutionContext, imageBuildInfo *config.ImageBuildInfo, target string, devContainerConfig *config.SubstitutedConfig, log log.Logger, forceBuild bool) (*ExtendedBuildInfo, error) {
	features, err := fetchFeatures(devContainerConfig.Config, log, forceBuild)
	if err != nil {
		return nil, fmt.Errorf("fetch features %w", err)
	}

	mergedImageMetadataConfig, err := metadata.GetDevContainerMetadata(ctx, imageBuildInfo.Metadata, devContainerConfig, features)
	if err != nil {
		return nil, fmt.Errorf("get dev container metadata %w", err)
	}

	marshalled, err := json.Marshal(mergedImageMetadataConfig.Raw)
	if err != nil {
		return nil, err
	}

	// no features?
	if len(features) == 0 {
		return &ExtendedBuildInfo{
			MetadataLabel:  string(marshalled),
			MetadataConfig: mergedImageMetadataConfig,
		}, nil
	}

	contextPath := config.GetContextPath(devContainerConfig.Config)
	buildInfo, err := getFeatureBuildOptions(contextPath, imageBuildInfo, target, features)
	if err != nil {
		return nil, err
	}

	return &ExtendedBuildInfo{
		Features:          features,
		FeaturesBuildInfo: buildInfo,
		MetadataConfig:    mergedImageMetadataConfig,
		MetadataLabel:     string(marshalled),
	}, nil
}

func getFeatureBuildOptions(contextPath string, imageBuildInfo *config.ImageBuildInfo, target string, features []*config.FeatureSet) (*BuildInfo, error) {
	containerUser, remoteUser := findContainerUsers(imageBuildInfo.Metadata, "", imageBuildInfo.User)

	// copy features
	featureFolder := filepath.Join(contextPath, config.DevPodContextFeatureFolder)
	err := copyFeaturesToDestination(features, featureFolder)
	if err != nil {
		return nil, err
	}

	// write devcontainer-features.builtin.env, its important to have a terminating \n here as we append to that file later
	err = os.WriteFile(filepath.Join(featureFolder, "devcontainer-features.builtin.env"), []byte(`_CONTAINER_USER=`+containerUser+`
_REMOTE_USER=`+remoteUser+"\n"), 0600)
	if err != nil {
		return nil, err
	}

	// prepare dockerfile
	dockerfileContent := strings.ReplaceAll(FEATURE_BASE_DOCKERFILE, "#{featureLayer}", getFeatureLayers(containerUser, remoteUser, features))
	// get build syntax from Dockerfile or use default
	syntax := "docker.io/docker/dockerfile:1.4"
	if imageBuildInfo.Dockerfile != nil && imageBuildInfo.Dockerfile.Syntax != "" {
		syntax = imageBuildInfo.Dockerfile.Syntax
	}
	dockerfilePrefix := fmt.Sprintf(`
# syntax=%s
ARG _DEV_CONTAINERS_BASE_IMAGE=placeholder`, syntax)

	return &BuildInfo{
		FeaturesFolder:          featureFolder,
		DockerfileContent:       dockerfileContent,
		DockerfilePrefixContent: dockerfilePrefix,
		OverrideTarget:          "dev_containers_target_stage",
		BuildArgs: map[string]string{
			"_DEV_CONTAINERS_BASE_IMAGE": target,
			"_DEV_CONTAINERS_IMAGE_USER": imageBuildInfo.User,
		},
	}, nil
}

func copyFeaturesToDestination(features []*config.FeatureSet, targetDir string) error {
	// make sure the folder doesn't exist initially
	_ = os.RemoveAll(targetDir)
	for i, feature := range features {
		featureDir := filepath.Join(targetDir, strconv.Itoa(i))
		err := os.MkdirAll(featureDir, 0755)
		if err != nil {
			return err
		}

		err = copy.Directory(feature.Folder, featureDir)
		if err != nil {
			return fmt.Errorf("copy feature %s %w", feature.ConfigID, err)
		}

		// copy feature folder
		envPath := filepath.Join(featureDir, "devcontainer-features.env")
		variables := getFeatureEnvVariables(feature.Config, feature.Options)
		err = os.WriteFile(envPath, []byte(strings.Join(variables, "\n")), 0600)
		if err != nil {
			return fmt.Errorf("write variables of feature %s %w", feature.ConfigID, err)
		}

		installWrapperPath := filepath.Join(featureDir, "devcontainer-features-install.sh")
		installWrapperContent := getFeatureInstallWrapperScript(feature.ConfigID, feature.Config, variables)
		err = os.WriteFile(installWrapperPath, []byte(installWrapperContent), 0600)
		if err != nil {
			return fmt.Errorf("write install wrapper script for feature %s %w", feature.ConfigID, err)
		}
	}

	return nil
}

func getFeatureSafeID(featureID string) string {
	return strings.ToUpper(featureSafeIDRegex2.ReplaceAllString(featureSafeIDRegex1.ReplaceAllString(featureID, "_"), "_"))
}

func getFeatureLayers(containerUser, remoteUser string, features []*config.FeatureSet) string {
	result := `RUN \
echo "_CONTAINER_USER_HOME=$(getent passwd ` + containerUser + ` | cut -d: -f6)" >> /tmp/build-features/devcontainer-features.builtin.env && \
echo "_REMOTE_USER_HOME=$(getent passwd ` + remoteUser + ` | cut -d: -f6)" >> /tmp/build-features/devcontainer-features.builtin.env

`
	for i, feature := range features {
		result += generateContainerEnvs(feature)
		result += `
RUN cd /tmp/build-features/` + strconv.Itoa(i) + ` \
&& chmod +x ./devcontainer-features-install.sh \
&& ./devcontainer-features-install.sh

`
	}

	return result
}

func generateContainerEnvs(feature *config.FeatureSet) string {
	result := []string{}
	if len(feature.Config.ContainerEnv) == 0 {
		return ""
	}

	for k, v := range feature.Config.ContainerEnv {
		result = append(result, fmt.Sprintf("ENV %s=%s", k, v))
	}
	return strings.Join(result, "\n")
}

func findContainerUsers(baseImageMetadata *config.ImageMetadataConfig, composeServiceUser, imageUser string) (string, string) {
	reversed := config.ReverseSlice(baseImageMetadata.Config)
	containerUser := ""
	remoteUser := ""
	for _, imageMetadata := range reversed {
		if containerUser == "" && imageMetadata.ContainerUser != "" {
			containerUser = imageMetadata.ContainerUser
		}
		if remoteUser == "" && imageMetadata.RemoteUser != "" {
			remoteUser = imageMetadata.RemoteUser
		}
	}

	if containerUser == "" {
		if composeServiceUser != "" {
			containerUser = composeServiceUser
		} else if imageUser != "" {
			containerUser = imageUser
		}
	}
	if remoteUser == "" {
		if composeServiceUser != "" {
			remoteUser = composeServiceUser
		} else if imageUser != "" {
			remoteUser = imageUser
		}
	}
	return containerUser, remoteUser
}

func fetchFeatures(devContainerConfig *config.DevContainerConfig, log log.Logger, forceBuild bool) ([]*config.FeatureSet, error) {
	// Process user-defined features first
	userFeatures := map[string]*config.FeatureSet{}
	for featureID, featureOptions := range devContainerConfig.Features {
		featureSet, err := processFeature(featureID, featureOptions, devContainerConfig, log, forceBuild)
		if err != nil {
			return nil, fmt.Errorf("process feature %s %w", featureID, err)
		}
		userFeatures[featureSet.ConfigID] = featureSet
	}

	// Resolve dependencies recursively
	allFeatures, err := resolveDependencies(devContainerConfig, userFeatures, log, forceBuild)
	if err != nil {
		return nil, fmt.Errorf("resolve dependencies %w", err)
	}

	// Convert map to slice
	featureSets := make([]*config.FeatureSet, 0, len(allFeatures))
	for _, featureSet := range allFeatures {
		featureSets = append(featureSets, featureSet)
	}

	featureSets, err = getSortedFeatureSets(devContainerConfig, featureSets)
	if err != nil {
		return nil, fmt.Errorf("failed to get sorted feature sets %w", err)
	}

	return featureSets, nil
}

func processFeature(featureID string, featureOptions any, devContainerConfig *config.DevContainerConfig, log log.Logger, forceBuild bool) (*config.FeatureSet, error) {
	featureFolder, err := ProcessFeatureID(featureID, devContainerConfig, log, forceBuild)
	if err != nil {
		return nil, fmt.Errorf("process feature ID %w", err)
	}

	log.Debugf("parse dev container feature in %s", featureFolder)
	featureConfig, err := config.ParseDevContainerFeature(featureFolder)
	if err != nil {
		return nil, fmt.Errorf("parse feature %w", err)
	}

	return &config.FeatureSet{
		ConfigID: NormalizeFeatureID(featureID),
		Folder:   featureFolder,
		Config:   featureConfig,
		Options:  featureOptions,
	}, nil
}

func resolveFeatureDependency(
	featureID string,
	featureSet *config.FeatureSet,
	features map[string]*config.FeatureSet,
	resolved map[string]*config.FeatureSet,
	visiting map[string]bool,
	devContainerConfig *config.DevContainerConfig,
	log log.Logger,
	forceBuild bool,
) error {
	if resolved[featureID] != nil {
		return nil // Already resolved
	}

	if visiting[featureID] {
		return fmt.Errorf("circular dependency detected involving feature %s", featureID)
	}

	visiting[featureID] = true
	defer func() { visiting[featureID] = false }()

	// Process dependencies first
	for depID, depOptions := range featureSet.Config.DependsOn {
		normalizedDepID := NormalizeFeatureID(depID)
		depFeatureSet, exists := features[normalizedDepID]
		if !exists {
			// Auto-install dependency
			log.Debugf("installing dependency feature %s", depID)
			var err error
			depFeatureSet, err = processFeature(depID, depOptions, devContainerConfig, log, forceBuild)
			if err != nil {
				return fmt.Errorf("failed to resolve dependency %s %w", depID, err)
			}
			features[normalizedDepID] = depFeatureSet
		}

		err := resolveFeatureDependency(normalizedDepID, depFeatureSet, features, resolved, visiting, devContainerConfig, log, forceBuild)
		if err != nil {
			return err
		}
	}

	resolved[featureID] = featureSet
	return nil
}

func resolveDependencies(
	devContainerConfig *config.DevContainerConfig,
	features map[string]*config.FeatureSet,
	log log.Logger,
	forceBuild bool,
) (map[string]*config.FeatureSet, error) {
	resolved := make(map[string]*config.FeatureSet)
	visiting := make(map[string]bool)

	// Resolve all features
	for featureID, featureSet := range features {
		err := resolveFeatureDependency(featureID, featureSet, features, resolved, visiting, devContainerConfig, log, forceBuild)
		if err != nil {
			return nil, err
		}
	}

	return resolved, nil
}

func NormalizeFeatureID(featureID string) string {
	ref, err := name.ParseReference(featureID)
	if err != nil {
		return featureID
	}

	tag, ok := ref.(name.Tag)
	if ok {
		return tag.Repository.Name()
	}

	return ref.String()
}

func getSortedFeatureSets(devContainer *config.DevContainerConfig, featureSets []*config.FeatureSet) ([]*config.FeatureSet, error) {
	orderedFeatureSets, err := getOrderedFeatureSets(featureSets)
	if err != nil {
		return nil, err
	}

	if len(devContainer.OverrideFeatureInstallOrder) == 0 {
		return orderedFeatureSets, nil
	}

	return sortFeaturesByOverride(devContainer.OverrideFeatureInstallOrder, orderedFeatureSets), nil
}

func sortFeaturesByOverride(overrideOrder []string, featureSets []*config.FeatureSet) []*config.FeatureSet {
	orderedFeatures := make([]*config.FeatureSet, 0, len(featureSets))
	seen := make(map[string]bool)

	for _, overrideFeatureID := range overrideOrder {
		feature := extractFeatureByID(featureSets, overrideFeatureID)
		if feature == nil {
			normalizedID := NormalizeFeatureID(overrideFeatureID)
			feature = extractFeatureByID(featureSets, normalizedID)
		}

		if feature != nil && !seen[feature.ConfigID] {
			orderedFeatures = append(orderedFeatures, feature)
			seen[feature.ConfigID] = true
		}
	}

	for _, feature := range featureSets {
		if !seen[feature.ConfigID] {
			orderedFeatures = append(orderedFeatures, feature)
			seen[feature.ConfigID] = true
		}
	}

	return orderedFeatures
}

func extractFeatureByID(features []*config.FeatureSet, featureID string) *config.FeatureSet {
	for _, feature := range features {
		if feature.ConfigID == featureID {
			return feature
		}
	}
	return nil
}

func containsFeature(features []*config.FeatureSet, featureID string) bool {
	for _, feature := range features {
		if feature.ConfigID == featureID {
			return true
		}
	}
	return false
}

func getOrderedFeatureSets(features []*config.FeatureSet) ([]*config.FeatureSet, error) {
	dependencyGraph, err := buildFeatureDependencyGraph(features)
	if err != nil {
		return nil, err
	}

	return dependencyGraph.Sort()
}

func buildFeatureDependencyGraph(features []*config.FeatureSet) (*graph.Graph[*config.FeatureSet], error) {
	g := graph.NewGraph[*config.FeatureSet]()
	featureLookup := buildFeatureLookupMap(features)
	nodes := make(map[string]*config.FeatureSet, len(features))
	for _, feature := range features {
		nodes[feature.ConfigID] = feature
	}
	if err := g.AddNodes(nodes); err != nil {
		return nil, fmt.Errorf("failed to add features: %w", err)
	}

	for _, feature := range features {
		if err := addHardDependencies(g, feature, featureLookup); err != nil {
			return nil, err
		}

		if err := addSoftDependencies(g, feature, featureLookup); err != nil {
			return nil, err
		}
	}

	return g, nil
}

func addHardDependencies(g *graph.Graph[*config.FeatureSet], feature *config.FeatureSet, featureLookup map[string]*config.FeatureSet) error {
	for id := range feature.Config.DependsOn {
		normalizedID := NormalizeFeatureID(id)
		if _, exists := featureLookup[normalizedID]; exists {
			if err := g.AddEdge(normalizedID, feature.ConfigID); err != nil {
				return err
			}
		}
	}
	return nil
}

func addSoftDependencies(g *graph.Graph[*config.FeatureSet], feature *config.FeatureSet, featureLookup map[string]*config.FeatureSet) error {
	for _, id := range feature.Config.InstallsAfter {
		normalizedID := NormalizeFeatureID(id)
		if _, exists := featureLookup[normalizedID]; !exists {
			continue
		}

		if hasHardDependency(feature, id, normalizedID) {
			continue // already added as hard dependency
		}

		if err := g.AddEdge(normalizedID, feature.ConfigID); err != nil {
			return err
		}
	}
	return nil
}

func buildFeatureLookupMap(features []*config.FeatureSet) map[string]*config.FeatureSet {
	lookup := make(map[string]*config.FeatureSet, len(features))
	for _, feature := range features {
		lookup[feature.ConfigID] = feature
	}
	return lookup
}

func hasHardDependency(feature *config.FeatureSet, originalID, normalizedID string) bool {
	_, existsOriginal := feature.Config.DependsOn[originalID]
	_, existsNormalized := feature.Config.DependsOn[normalizedID]
	return existsOriginal || existsNormalized
}
