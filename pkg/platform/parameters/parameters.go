package parameters

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	storagev1 "github.com/loft-sh/api/v4/pkg/apis/storage/v1"
)

func VerifyValue(value string, parameter storagev1.AppParameter) (any, error) {
	switch parameter.Type {
	case "":
		fallthrough
	case "password":
		fallthrough
	case "string":
		fallthrough
	case "multiline":
		if parameter.DefaultValue != "" && value == "" {
			value = parameter.DefaultValue
		}

		if parameter.Required && value == "" {
			return nil, fmt.Errorf("parameter %s (%s) is required", parameter.Label, parameter.Variable)
		}
		if slices.Contains(parameter.Options, value) {
			return value, nil
		}
		if parameter.Validation != "" {
			regEx, err := regexp.Compile(parameter.Validation)
			if err != nil {
				return nil, fmt.Errorf("compile validation regex %s: %w", parameter.Validation, err)
			}

			if !regEx.MatchString(value) {
				return nil, fmt.Errorf("parameter %s (%s) needs to match regex %s", parameter.Label, parameter.Variable, parameter.Validation)
			}
		}
		if parameter.Invalidation != "" {
			regEx, err := regexp.Compile(parameter.Invalidation)
			if err != nil {
				return nil, fmt.Errorf("compile invalidation regex %s: %w", parameter.Invalidation, err)
			}

			if regEx.MatchString(value) {
				return nil, fmt.Errorf("parameter %s (%s) cannot match regex %s", parameter.Label, parameter.Variable, parameter.Invalidation)
			}
		}

		return value, nil
	case "boolean":
		if parameter.DefaultValue != "" && value == "" {
			boolValue, err := strconv.ParseBool(parameter.DefaultValue)
			if err != nil {
				return nil, fmt.Errorf("parse default value for parameter %s (%s): %w", parameter.Label, parameter.Variable, err)
			}

			return boolValue, nil
		}
		if parameter.Required && value == "" {
			return nil, fmt.Errorf("parameter %s (%s) is required", parameter.Label, parameter.Variable)
		}

		boolValue, err := strconv.ParseBool(value)
		if err != nil {
			return nil, fmt.Errorf("parse value for parameter %s (%s): %w", parameter.Label, parameter.Variable, err)
		}
		return boolValue, nil
	case "number":
		if parameter.DefaultValue != "" && value == "" {
			intValue, err := strconv.Atoi(parameter.DefaultValue)
			if err != nil {
				return nil, fmt.Errorf("parse default value for parameter %s (%s): %w", parameter.Label, parameter.Variable, err)
			}

			return intValue, nil
		}
		if parameter.Required && value == "" {
			return nil, fmt.Errorf("parameter %s (%s) is required", parameter.Label, parameter.Variable)
		}
		num, err := strconv.Atoi(value)
		if err != nil {
			return nil, fmt.Errorf("parse value for parameter %s (%s): %w", parameter.Label, parameter.Variable, err)
		}
		if parameter.Min != nil && num < *parameter.Min {
			return nil, fmt.Errorf("parameter %s (%s) cannot be smaller than %d", parameter.Label, parameter.Variable, *parameter.Min)
		}
		if parameter.Max != nil && num > *parameter.Max {
			return nil, fmt.Errorf("parameter %s (%s) cannot be greater than %d", parameter.Label, parameter.Variable, *parameter.Max)
		}

		return num, nil
	}

	return nil, fmt.Errorf("unrecognized type %s for parameter %s (%s)", parameter.Type, parameter.Label, parameter.Variable)
}

func GetDeepValue(parameters any, path string) any {
	if parameters == nil {
		return nil
	}

	pathSegments := strings.Split(path, ".")
	switch t := parameters.(type) {
	case map[string]any:
		val, ok := t[pathSegments[0]]
		if !ok {
			return nil
		} else if len(pathSegments) == 1 {
			return val
		}

		return GetDeepValue(val, strings.Join(pathSegments[1:], "."))
	case []any:
		index, err := strconv.Atoi(pathSegments[0])
		if err != nil {
			return nil
		} else if index < 0 || index >= len(t) {
			return nil
		}

		val := t[index]
		if len(pathSegments) == 1 {
			return val
		}

		return GetDeepValue(val, strings.Join(pathSegments[1:], "."))
	}

	return nil
}
