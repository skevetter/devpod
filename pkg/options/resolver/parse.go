package resolver

import (
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/loft-sh/log"
	"github.com/skevetter/devpod/pkg/config"
	"github.com/skevetter/devpod/pkg/types"
)

func printUnusedUserValues(userValues map[string]string, options config.OptionDefinitions, log log.Logger) {
	allowedOptions := []string{}
	for k := range options {
		allowedOptions = append(allowedOptions, k)
	}

	for k := range userValues {
		if options[k] == nil {
			log.Warnf("Option %s was specified but is not defined, allowed options are %v", k, allowedOptions)
		}
	}
}

func validateUserValue(optionName, userValue string, option *types.Option) error {
	if option.ValidationPattern != "" {
		matcher, err := regexp.Compile(option.ValidationPattern)
		if err != nil {
			return err
		}

		if !matcher.MatchString(userValue) {
			if option.ValidationMessage != "" {
				return fmt.Errorf("%s", option.ValidationMessage)
			}

			return fmt.Errorf("invalid value '%s' for option '%s', has to match the following regEx: %s", userValue, optionName, option.ValidationPattern)
		}
	}

	if len(option.Enum) > 0 {
		found := false
		for _, e := range option.Enum {
			if userValue == e.Value {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("invalid value '%s' for option '%s', has to match one of the following values: %v", userValue, optionName, option.Enum)
		}
	}

	if option.Type != "" {
		switch option.Type {
		case "number":
			_, err := strconv.ParseInt(userValue, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid value '%s' for option '%s', must be a number", userValue, optionName)
			}
		case "boolean":
			_, err := strconv.ParseBool(userValue)
			if err != nil {
				return fmt.Errorf("invalid value '%s' for option '%s', must be a boolean", userValue, optionName)
			}
		case "duration":
			_, err := time.ParseDuration(userValue)
			if err != nil {
				return fmt.Errorf("invalid value '%s' for option '%s', must be a duration like 10s, 5m or 24h", userValue, optionName)
			}
		}
	}

	return nil
}
