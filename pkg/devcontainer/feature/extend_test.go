package feature

import (
	"strings"
	"testing"

	"github.com/skevetter/devpod/pkg/devcontainer/config"
)

func TestCreateFeatureLookup(t *testing.T) {
	features := []*config.FeatureSet{
		{ConfigID: "feature-a"},
		{ConfigID: "feature-b"},
		{ConfigID: "feature-c"},
	}

	lookup := buildFeatureLookupMap(features)

	if len(lookup) != 3 {
		t.Errorf("Expected 3 entries, got %d", len(lookup))
	}

	for _, feature := range features {
		if lookup[feature.ConfigID] != feature {
			t.Errorf("Lookup failed for %s", feature.ConfigID)
		}
	}
}

func TestIsAlreadyHardDependency(t *testing.T) {
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
		t.Run(testCase.name, func(t *testing.T) {
			actualIsDuplicate := dependencyExists(testCase.feature, testCase.originalID, testCase.normalizedID)
			if actualIsDuplicate != testCase.expectedIsDuplicate {
				t.Errorf("Expected %v, got %v", testCase.expectedIsDuplicate, actualIsDuplicate)
			}
		})
	}
}

func TestComputeAutomaticFeatureOrder_SimpleDependency(t *testing.T) {
	features := []*config.FeatureSet{
		{
			ConfigID: "dependent-feature",
			Config: &config.FeatureConfig{
				DependsOn: config.DependsOnField{
					"dependency-feature": map[string]any{},
				},
			},
		},
		{
			ConfigID: "dependency-feature",
			Config: &config.FeatureConfig{
				DependsOn: config.DependsOnField{},
			},
		},
	}

	installationOrder, err := getOrderedFeatureSets(features)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(installationOrder) != 2 {
		t.Fatalf("Expected 2 features, got %d", len(installationOrder))
	}

	if installationOrder[0].ConfigID != "dependency-feature" {
		t.Errorf("Expected dependency-feature first, got %s", installationOrder[0].ConfigID)
	}
	if installationOrder[1].ConfigID != "dependent-feature" {
		t.Errorf("Expected dependent-feature second, got %s", installationOrder[1].ConfigID)
	}
}

func TestComputeAutomaticFeatureOrder_DependsOnAndInstallsAfter(t *testing.T) {
	features := []*config.FeatureSet{
		{
			ConfigID: NormalizeFeatureID("feature-with-both-dependencies"),
			Config: &config.FeatureConfig{
				DependsOn: config.DependsOnField{
					"shared-dependency": map[string]any{},
				},
				InstallsAfter: []string{"shared-dependency"},
			},
		},
		{
			ConfigID: NormalizeFeatureID("shared-dependency"),
			Config: &config.FeatureConfig{
				DependsOn:     config.DependsOnField{},
				InstallsAfter: []string{},
			},
		},
	}

	installationOrder, err := getOrderedFeatureSets(features)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(installationOrder) != 2 {
		t.Fatalf("Expected 2 features, got %d", len(installationOrder))
	}

	expectedSharedDep := NormalizeFeatureID("shared-dependency")
	expectedFeatureWithBoth := NormalizeFeatureID("feature-with-both-dependencies")

	if installationOrder[0].ConfigID != expectedSharedDep {
		t.Errorf("Expected %s first, got %s", expectedSharedDep, installationOrder[0].ConfigID)
	}
	if installationOrder[1].ConfigID != expectedFeatureWithBoth {
		t.Errorf("Expected %s second, got %s", expectedFeatureWithBoth, installationOrder[1].ConfigID)
	}
}

func TestComputeAutomaticFeatureOrder_OnlyInstallsAfter(t *testing.T) {
	features := []*config.FeatureSet{
		{
			ConfigID: "feature-with-soft-dependency",
			Config: &config.FeatureConfig{
				DependsOn:     config.DependsOnField{},
				InstallsAfter: []string{"preferred-first-feature"},
			},
		},
		{
			ConfigID: "preferred-first-feature",
			Config: &config.FeatureConfig{
				DependsOn:     config.DependsOnField{},
				InstallsAfter: []string{},
			},
		},
	}

	installationOrder, err := getOrderedFeatureSets(features)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(installationOrder) != 2 {
		t.Fatalf("Expected 2 features, got %d", len(installationOrder))
	}

	if installationOrder[0].ConfigID != "preferred-first-feature" {
		t.Errorf("Expected preferred-first-feature first, got %s", installationOrder[0].ConfigID)
	}
	if installationOrder[1].ConfigID != "feature-with-soft-dependency" {
		t.Errorf("Expected feature-with-soft-dependency second, got %s", installationOrder[1].ConfigID)
	}
}

func TestComputeAutomaticFeatureOrder_ChainedDependencies(t *testing.T) {
	features := []*config.FeatureSet{
		{
			ConfigID: "top-level-feature",
			Config: &config.FeatureConfig{
				DependsOn: config.DependsOnField{
					"middle-level-feature": map[string]any{},
				},
			},
		},
		{
			ConfigID: "middle-level-feature",
			Config: &config.FeatureConfig{
				DependsOn: config.DependsOnField{
					"base-level-feature": map[string]any{},
				},
			},
		},
		{
			ConfigID: "base-level-feature",
			Config: &config.FeatureConfig{
				DependsOn: config.DependsOnField{},
			},
		},
	}

	installationOrder, err := getOrderedFeatureSets(features)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(installationOrder) != 3 {
		t.Fatalf("Expected 3 features, got %d", len(installationOrder))
	}

	expectedOrder := []string{"base-level-feature", "middle-level-feature", "top-level-feature"}
	for i, expectedFeatureID := range expectedOrder {
		if installationOrder[i].ConfigID != expectedFeatureID {
			t.Errorf("Position %d: expected %s, got %s", i, expectedFeatureID, installationOrder[i].ConfigID)
		}
	}
}

func TestComputeAutomaticFeatureOrder_CircularDependency(t *testing.T) {
	features := []*config.FeatureSet{
		{
			ConfigID: NormalizeFeatureID("feature-a"),
			Config: &config.FeatureConfig{
				DependsOn: config.DependsOnField{
					"feature-b": map[string]any{},
				},
			},
		},
		{
			ConfigID: NormalizeFeatureID("feature-b"),
			Config: &config.FeatureConfig{
				DependsOn: config.DependsOnField{
					"feature-a": map[string]any{},
				},
			},
		},
	}

	_, err := getOrderedFeatureSets(features)
	if err == nil {
		t.Fatal("Expected circular dependency error, got nil")
	}

	if !strings.Contains(err.Error(), "circular") {
		t.Errorf("Expected circular dependency error, got: %v", err)
	}
}

func TestFeatureOrderWithDependencies_SameDependsOnAndInstallsAfter(t *testing.T) {
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
	if err != nil {
		t.Fatalf("Expected no circular dependency error, got: %v", err)
	}

	if len(installationOrder) != 2 {
		t.Fatalf("Expected 2 features, got %d", len(installationOrder))
	}

	if installationOrder[0].ConfigID != "ghcr.io/devcontainers/features/node" {
		t.Errorf("Expected node feature first, got %s", installationOrder[0].ConfigID)
	}
	if installationOrder[1].ConfigID != "dev-code" {
		t.Errorf("Expected dev-code second, got %s", installationOrder[1].ConfigID)
	}
}

func TestComputeFeatureOrder_NoOverride(t *testing.T) {
	devContainer := &config.DevContainerConfig{
		DevContainerConfigBase: config.DevContainerConfigBase{
			OverrideFeatureInstallOrder: []string{},
		},
	}

	features := []*config.FeatureSet{
		{ConfigID: NormalizeFeatureID("feature-a"), Config: &config.FeatureConfig{DependsOn: config.DependsOnField{"feature-b": map[string]any{}}}},
		{ConfigID: NormalizeFeatureID("feature-b"), Config: &config.FeatureConfig{DependsOn: config.DependsOnField{}}},
	}

	order, err := getSortedFeatureSets(devContainer, features)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(order) != 2 {
		t.Fatalf("Expected 2 features, got %d", len(order))
	}

	expectedFeatureB := NormalizeFeatureID("feature-b")
	expectedFeatureA := NormalizeFeatureID("feature-a")

	if order[0].ConfigID != expectedFeatureB || order[1].ConfigID != expectedFeatureA {
		t.Errorf("Expected [%s, %s], got [%s, %s]", expectedFeatureB, expectedFeatureA, order[0].ConfigID, order[1].ConfigID)
	}
}

func TestComputeFeatureOrder_WithOverride(t *testing.T) {
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
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(order) != 2 {
		t.Fatalf("Expected 2 features, got %d", len(order))
	}

	if order[0].ConfigID != "feature-a" || order[1].ConfigID != "feature-b" {
		t.Errorf("Expected [feature-a, feature-b], got [%s, %s]", order[0].ConfigID, order[1].ConfigID)
	}
}

func TestComputeFeatureOrder_PartialOverride(t *testing.T) {
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
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(order) != 3 {
		t.Fatalf("Expected 3 features, got %d", len(order))
	}

	if order[0].ConfigID != "feature-c" {
		t.Errorf("Expected feature-c first, got %s", order[0].ConfigID)
	}
}

func TestApplyManualOrdering(t *testing.T) {
	automaticOrder := []*config.FeatureSet{
		{ConfigID: "feature-a"},
		{ConfigID: "feature-b"},
		{ConfigID: "feature-c"},
	}

	overrideOrder := []string{"feature-c", "feature-a"}

	result := sortFeaturesByOverride(overrideOrder, automaticOrder)

	expected := []string{"feature-c", "feature-a", "feature-b"}
	if len(result) != 3 {
		t.Fatalf("Expected 3 features, got %d", len(result))
	}

	for i, expectedID := range expected {
		if result[i].ConfigID != expectedID {
			t.Errorf("Position %d: expected %s, got %s", i, expectedID, result[i].ConfigID)
		}
	}
}

func TestExtractFeatureByID(t *testing.T) {
	features := []*config.FeatureSet{
		{ConfigID: "feature-a"},
		{ConfigID: "feature-b"},
	}

	found := extractFeatureByID(features, "feature-b")
	if found == nil || found.ConfigID != "feature-b" {
		t.Errorf("Expected to find feature-b")
	}

	notFound := extractFeatureByID(features, "feature-c")
	if notFound != nil {
		t.Errorf("Expected not to find feature-c")
	}
}

func TestContainsFeature(t *testing.T) {
	features := []*config.FeatureSet{
		{ConfigID: "feature-a"},
		{ConfigID: "feature-b"},
	}

	if !containsFeature(features, "feature-a") {
		t.Errorf("Expected to contain feature-a")
	}

	if containsFeature(features, "feature-c") {
		t.Errorf("Expected not to contain feature-c")
	}
}
