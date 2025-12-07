package feature

import (
	"fmt"
	"maps"
	"sort"

	"github.com/skevetter/devpod/pkg/devcontainer/config"
)

func getFeatureEnvVariables(feature *config.FeatureConfig, featureOptions any) []string {
	options := getFeatureValueObject(feature, featureOptions)
	variables := []string{}
	for k, v := range options {
		variables = append(variables, fmt.Sprintf(`%s="%v"`, getFeatureSafeID(k), v))
	}

	sort.Strings(variables)

	return variables
}

func getFeatureValueObject(feature *config.FeatureConfig, featureOptions any) map[string]any {
	defaults := getFeatureDefaults(feature)
	switch t := featureOptions.(type) {
	case map[string]any:
		maps.Copy(defaults, t)

		return defaults
	case string:
		if feature.Options == nil {
			return defaults
		}

		_, ok := feature.Options["version"]
		if ok {
			defaults["version"] = t
		}

		return defaults
	}

	return defaults
}

func getFeatureDefaults(feature *config.FeatureConfig) map[string]any {
	ret := map[string]any{}
	for k, v := range feature.Options {
		ret[k] = string(v.Default)
	}

	return ret
}
