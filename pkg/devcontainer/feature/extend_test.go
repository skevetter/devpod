package feature

import (
	"testing"

	"github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/stretchr/testify/suite"
)

type ExtendTestSuite struct {
	suite.Suite
}

func TestExtendTestSuite(t *testing.T) {
	suite.Run(t, new(ExtendTestSuite))
}

func (suite *ExtendTestSuite) TestCreateFeatureLookup() {
	features := []*config.FeatureSet{
		{ConfigID: "feature-a"},
		{ConfigID: "feature-b"},
		{ConfigID: "feature-c"},
	}

	lookup := buildFeatureLookupMap(features)
	suite.Len(lookup, 3)

	for _, feature := range features {
		suite.Equal(feature, lookup[feature.ConfigID])
	}
}

func (suite *ExtendTestSuite) TestHasHardDependency() {
	tests := []struct {
		name                string
		feature             *config.FeatureSet
		originalID          string
		normalizedID        string
		expectedIsDuplicate bool
	}{
		{
			name: "exact match in dependsOn",
			feature: &config.FeatureSet{
				Config: &config.FeatureConfig{
					DependsOn: config.DependsOnField{
						"node": map[string]any{},
					},
				},
			},
			originalID:          "node",
			normalizedID:        "node",
			expectedIsDuplicate: true,
		},
		{
			name: "normalized match in dependsOn",
			feature: &config.FeatureSet{
				Config: &config.FeatureConfig{
					DependsOn: config.DependsOnField{
						"ghcr.io/devcontainers/features/node": map[string]any{},
					},
				},
			},
			originalID:          "ghcr.io/devcontainers/features/node:latest",
			normalizedID:        "ghcr.io/devcontainers/features/node",
			expectedIsDuplicate: true,
		},
		{
			name: "no match",
			feature: &config.FeatureSet{
				Config: &config.FeatureConfig{
					DependsOn: config.DependsOnField{
						"python": map[string]any{},
					},
				},
			},
			originalID:          "node",
			normalizedID:        "node",
			expectedIsDuplicate: false,
		},
		{
			name: "empty dependsOn",
			feature: &config.FeatureSet{
				Config: &config.FeatureConfig{
					DependsOn: config.DependsOnField{},
				},
			},
			originalID:          "node",
			normalizedID:        "node",
			expectedIsDuplicate: false,
		},
	}

	for _, testCase := range tests {
		suite.Run(testCase.name, func() {
			actualIsDuplicate := hasHardDependency(testCase.feature, testCase.originalID, testCase.normalizedID)
			suite.Equal(testCase.expectedIsDuplicate, actualIsDuplicate)
		})
	}
}

func (suite *ExtendTestSuite) TestComputeAutomaticFeatureOrder_SimpleDependency() {
	features := []*config.FeatureSet{
		{
			ConfigID: normalizeFeatureID("dependent-feature"),
			Config: &config.FeatureConfig{
				DependsOn: config.DependsOnField{
					"dependency-feature": map[string]any{},
				},
			},
		},
		{
			ConfigID: normalizeFeatureID("dependency-feature"),
			Config: &config.FeatureConfig{
				DependsOn: config.DependsOnField{},
			},
		},
	}

	installationOrder, err := getOrderedFeatureSets(features)
	suite.Require().NoError(err)

	suite.Len(installationOrder, 2)
	expectedDependency := normalizeFeatureID("dependency-feature")
	expectedDependent := normalizeFeatureID("dependent-feature")

	suite.Equal(expectedDependency, installationOrder[0].ConfigID)
	suite.Equal(expectedDependent, installationOrder[1].ConfigID)
}

func (suite *ExtendTestSuite) TestComputeAutomaticFeatureOrder_DependsOnAndInstallsAfter() {
	features := []*config.FeatureSet{
		{
			ConfigID: normalizeFeatureID("feature-with-both-dependencies"),
			Config: &config.FeatureConfig{
				DependsOn: config.DependsOnField{
					"shared-dependency": map[string]any{},
				},
				InstallsAfter: []string{"shared-dependency"},
			},
		},
		{
			ConfigID: normalizeFeatureID("shared-dependency"),
			Config: &config.FeatureConfig{
				DependsOn:     config.DependsOnField{},
				InstallsAfter: []string{},
			},
		},
	}

	installationOrder, err := getOrderedFeatureSets(features)
	suite.Require().NoError(err)

	suite.Len(installationOrder, 2)
	expectedSharedDep := normalizeFeatureID("shared-dependency")
	expectedFeatureWithBoth := normalizeFeatureID("feature-with-both-dependencies")

	suite.Equal(expectedSharedDep, installationOrder[0].ConfigID)
	suite.Equal(expectedFeatureWithBoth, installationOrder[1].ConfigID)
}

func (suite *ExtendTestSuite) TestComputeAutomaticFeatureOrder_OnlyInstallsAfter() {
	features := []*config.FeatureSet{
		{
			ConfigID: normalizeFeatureID("feature-with-soft-dependency"),
			Config: &config.FeatureConfig{
				DependsOn:     config.DependsOnField{},
				InstallsAfter: []string{"preferred-first-feature"},
			},
		},
		{
			ConfigID: normalizeFeatureID("preferred-first-feature"),
			Config: &config.FeatureConfig{
				DependsOn:     config.DependsOnField{},
				InstallsAfter: []string{},
			},
		},
	}

	installationOrder, err := getOrderedFeatureSets(features)
	suite.Require().NoError(err)

	suite.Len(installationOrder, 2)
	expectedPreferredFirst := normalizeFeatureID("preferred-first-feature")
	expectedFeatureWithSoft := normalizeFeatureID("feature-with-soft-dependency")

	suite.Equal(expectedPreferredFirst, installationOrder[0].ConfigID)
	suite.Equal(expectedFeatureWithSoft, installationOrder[1].ConfigID)
}

func (suite *ExtendTestSuite) TestComputeAutomaticFeatureOrder_ChainedDependencies() {
	features := []*config.FeatureSet{
		{
			ConfigID: normalizeFeatureID("top-level-feature"),
			Config: &config.FeatureConfig{
				DependsOn: config.DependsOnField{
					"middle-level-feature": map[string]any{},
				},
			},
		},
		{
			ConfigID: normalizeFeatureID("middle-level-feature"),
			Config: &config.FeatureConfig{
				DependsOn: config.DependsOnField{
					"base-level-feature": map[string]any{},
				},
			},
		},
		{
			ConfigID: normalizeFeatureID("base-level-feature"),
			Config: &config.FeatureConfig{
				DependsOn: config.DependsOnField{},
			},
		},
	}

	installationOrder, err := getOrderedFeatureSets(features)
	suite.Require().NoError(err)

	suite.Len(installationOrder, 3)

	expectedOrder := []string{
		normalizeFeatureID("base-level-feature"),
		normalizeFeatureID("middle-level-feature"),
		normalizeFeatureID("top-level-feature"),
	}
	for i, expectedFeatureID := range expectedOrder {
		if installationOrder[i].ConfigID != expectedFeatureID {
			suite.Failf("Position mismatch", "Position %d: expected %s, got %s", i, expectedFeatureID, installationOrder[i].ConfigID)
		}
	}
}

func (suite *ExtendTestSuite) TestComputeAutomaticFeatureOrder_CircularDependency() {
	features := []*config.FeatureSet{
		{
			ConfigID: normalizeFeatureID("feature-a"),
			Config: &config.FeatureConfig{
				DependsOn: config.DependsOnField{
					"feature-b": map[string]any{},
				},
			},
		},
		{
			ConfigID: normalizeFeatureID("feature-b"),
			Config: &config.FeatureConfig{
				DependsOn: config.DependsOnField{
					"feature-a": map[string]any{},
				},
			},
		},
	}

	_, err := getOrderedFeatureSets(features)
	suite.Error(err)
	suite.Contains(err.Error(), "circular")
}

func (suite *ExtendTestSuite) TestFeatureOrderWithDependencies_SameDependsOnAndInstallsAfter() {
	features := []*config.FeatureSet{
		{
			ConfigID: "dev-code",
			Config: &config.FeatureConfig{
				DependsOn: config.DependsOnField{
					"ghcr.io/devcontainers/features/node": map[string]any{},
				},
				InstallsAfter: []string{"ghcr.io/devcontainers/features/node"},
			},
		},
		{
			ConfigID: "ghcr.io/devcontainers/features/node",
			Config: &config.FeatureConfig{
				DependsOn:     config.DependsOnField{},
				InstallsAfter: []string{},
			},
		},
	}

	installationOrder, err := getOrderedFeatureSets(features)
	suite.Require().NoError(err)
	suite.Len(installationOrder, 2)
	suite.Equal("ghcr.io/devcontainers/features/node", installationOrder[0].ConfigID)
	suite.Equal("dev-code", installationOrder[1].ConfigID)
}

func (suite *ExtendTestSuite) TestComputeFeatureOrder_NoOverride() {
	devContainer := &config.DevContainerConfig{
		DevContainerConfigBase: config.DevContainerConfigBase{
			OverrideFeatureInstallOrder: []string{},
		},
	}

	features := []*config.FeatureSet{
		{ConfigID: normalizeFeatureID("feature-a"), Config: &config.FeatureConfig{DependsOn: config.DependsOnField{"feature-b": map[string]any{}}}},
		{ConfigID: normalizeFeatureID("feature-b"), Config: &config.FeatureConfig{DependsOn: config.DependsOnField{}}},
	}

	order, err := getSortedFeatureSets(devContainer, features)
	suite.Require().NoError(err)

	suite.Len(order, 2)
	expectedFeatureB := normalizeFeatureID("feature-b")
	expectedFeatureA := normalizeFeatureID("feature-a")
	if order[0].ConfigID != expectedFeatureB || order[1].ConfigID != expectedFeatureA {
		suite.Failf("Order mismatch", "Expected [%s, %s], got [%s, %s]", expectedFeatureB, expectedFeatureA, order[0].ConfigID, order[1].ConfigID)
	}
}

func (suite *ExtendTestSuite) TestComputeFeatureOrder_WithOverride() {
	devContainer := &config.DevContainerConfig{
		DevContainerConfigBase: config.DevContainerConfigBase{
			OverrideFeatureInstallOrder: []string{"feature-a", "feature-b"},
		},
	}

	features := []*config.FeatureSet{
		{ConfigID: "feature-a", Config: &config.FeatureConfig{DependsOn: config.DependsOnField{"feature-b": map[string]any{}}}},
		{ConfigID: "feature-b", Config: &config.FeatureConfig{DependsOn: config.DependsOnField{}}},
	}

	order, err := getSortedFeatureSets(devContainer, features)
	suite.Require().NoError(err)
	suite.Len(order, 2)
	if order[0].ConfigID != "feature-a" || order[1].ConfigID != "feature-b" {
		suite.Failf("Order mismatch", "Expected [feature-a, feature-b], got [%s, %s]", order[0].ConfigID, order[1].ConfigID)
	}
}

func (suite *ExtendTestSuite) TestComputeFeatureOrder_PartialOverride() {
	devContainer := &config.DevContainerConfig{
		DevContainerConfigBase: config.DevContainerConfigBase{
			OverrideFeatureInstallOrder: []string{"feature-c"},
		},
	}

	features := []*config.FeatureSet{
		{ConfigID: "feature-a", Config: &config.FeatureConfig{DependsOn: config.DependsOnField{"feature-b": map[string]any{}}}},
		{ConfigID: "feature-b", Config: &config.FeatureConfig{DependsOn: config.DependsOnField{}}},
		{ConfigID: "feature-c", Config: &config.FeatureConfig{DependsOn: config.DependsOnField{}}},
	}

	order, err := getSortedFeatureSets(devContainer, features)
	suite.Require().NoError(err)
	suite.Len(order, 3)

	if order[0].ConfigID != "feature-c" {
		suite.Failf("First element mismatch", "Expected feature-c first, got %s", order[0].ConfigID)
	}
}

func (suite *ExtendTestSuite) TestApplyManualOrdering() {
	automaticOrder := []*config.FeatureSet{
		{ConfigID: "feature-a"},
		{ConfigID: "feature-b"},
		{ConfigID: "feature-c"},
	}

	overrideOrder := []string{"feature-c", "feature-a"}
	result := sortFeaturesByOverride(overrideOrder, automaticOrder)
	expected := []string{"feature-c", "feature-a", "feature-b"}
	suite.Len(result, 3)
	for i, expectedID := range expected {
		if result[i].ConfigID != expectedID {
			suite.Failf("Position mismatch", "Position %d: expected %s, got %s", i, expectedID, result[i].ConfigID)
		}
	}
}

func (suite *ExtendTestSuite) TestExtractFeatureByID() {
	features := []*config.FeatureSet{
		{ConfigID: "feature-a"},
		{ConfigID: "feature-b"},
	}

	found := extractFeatureByID(features, "feature-b")
	if found == nil || found.ConfigID != "feature-b" {
		suite.Fail("Expected to find feature-b")
	}

	notFound := extractFeatureByID(features, "feature-c")
	if notFound != nil {
		suite.Fail("Expected not to find feature-c")
	}
}

func (suite *ExtendTestSuite) TestContainsFeature() {
	features := []*config.FeatureSet{
		{ConfigID: "feature-a"},
		{ConfigID: "feature-b"},
	}

	if !containsFeature(features, "feature-a") {
		suite.Fail("Expected to contain feature-a")
	}

	if containsFeature(features, "feature-c") {
		suite.Fail("Expected not to contain feature-c")
	}
}

func (suite *ExtendTestSuite) TestResolveFeatureUsersUsesMergedMetadata() {
	imageBuildInfo := &config.ImageBuildInfo{
		User: "nonroot",
		Metadata: &config.ImageMetadataConfig{
			Config: []*config.ImageMetadata{{
				DevContainerConfigBase: config.DevContainerConfigBase{
					RemoteUser: "nonroot",
				},
			}},
		},
	}

	mergedImageMetadata := &config.ImageMetadataConfig{
		Config: []*config.ImageMetadata{{
			DevContainerConfigBase: config.DevContainerConfigBase{
				RemoteUser: "vscode",
			},
		}},
	}

	containerUser, remoteUser := resolveFeatureUsers(imageBuildInfo, mergedImageMetadata)
	suite.Equal("nonroot", containerUser)
	suite.Equal("vscode", remoteUser)
}

func (suite *ExtendTestSuite) TestResolveFeatureUsersFallsBackToImageBuildInfoMetadata() {
	imageBuildInfo := &config.ImageBuildInfo{
		User: "root",
		Metadata: &config.ImageMetadataConfig{
			Config: []*config.ImageMetadata{{
				NonComposeBase: config.NonComposeBase{ContainerUser: "appuser"},
				DevContainerConfigBase: config.DevContainerConfigBase{
					RemoteUser: "devuser",
				},
			}},
		},
	}

	containerUser, remoteUser := resolveFeatureUsers(imageBuildInfo, nil)
	suite.Equal("appuser", containerUser)
	suite.Equal("devuser", remoteUser)
}
