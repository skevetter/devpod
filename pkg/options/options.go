package options

import (
	"os"
	"strings"
)

func AssignUnassignedFromEnvironment(
	assignments []string,
	names []string,
	environmentVariablePrefix string,
) []string {
	var result = assignments
	for _, name := range names {
		if value, exists := os.LookupEnv(environmentVariablePrefix + name); exists && !isAssigned(assignments, name) {
			result = append(result, name+"="+value)
		}
	}
	return result
}

func isAssigned(assignments []string, name string) bool {
	for _, assignment := range assignments {
		if strings.HasPrefix(assignment, name+"=") {
			return true
		}
	}
	return false
}
