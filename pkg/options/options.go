package options

import (
	"os"
	"strings"
)

// Takes a list of assignments in KEY=VALUE format, a list of option names to check, and an environment variable prefix,
// and returns a new list with additional assignments from environment variables for any names not already assigned.
func PropagateFromEnvironment(
	assignments []string,
	names []string,
	prefix string,
) []string {
	assigned := assignedNames(assignments)

	result := assignments
	for _, name := range names {
		if assigned[name] {
			continue
		}
		if value, exists := os.LookupEnv(prefix + name); exists {
			result = append(result, name+"="+value)
		}
	}
	return result
}

func assignedNames(assignments []string) map[string]bool {
	names := make(map[string]bool, len(assignments))
	for _, assignment := range assignments {
		if idx := strings.Index(assignment, "="); idx != -1 {
			names[assignment[:idx]] = true
		}
	}
	return names
}
