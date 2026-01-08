package options

import (
	"fmt"
	"os"
	"testing"

	"gotest.tools/assert"
)

type assignmentTestCase struct {
	Name string

	Names                     []string
	Assignments               []string
	EnvironmentVariablePrefix string
	NotInEnvironment          []string
	Environment               map[string]string

	ExpectedAssignments []string
}

func TestPropagateFromEnvironment(t *testing.T) {
	testCases := []assignmentTestCase{
		{
			Name: "assigned, not in the environment",
			Names: []string{
				"HOST",
			},
			Assignments: []string{
				"HOST=box",
			},
			EnvironmentVariablePrefix: "DEVPOD_PROVIDER_SSH_",
			NotInEnvironment: []string{
				"DEVPOD_PROVIDER_SSH_HOST",
			},
			Environment: map[string]string{},
			ExpectedAssignments: []string{
				"HOST=box",
			},
		},
		{
			Name: "not assigned, not in the environment",
			Names: []string{
				"HOST",
			},
			Assignments:               []string{},
			EnvironmentVariablePrefix: "DEVPOD_PROVIDER_SSH_",
			NotInEnvironment: []string{
				"DEVPOD_PROVIDER_SSH_HOST",
			},
			Environment:         map[string]string{},
			ExpectedAssignments: []string{},
		},
		{
			Name: "assigned, in the environment",
			Names: []string{
				"HOST",
			},
			Assignments: []string{
				"HOST=box",
			},
			EnvironmentVariablePrefix: "DEVPOD_PROVIDER_SSH_",
			NotInEnvironment:          []string{},
			Environment: map[string]string{
				"DEVPOD_PROVIDER_SSH_HOST": "another-box",
			},
			ExpectedAssignments: []string{
				"HOST=box",
			},
		},
		{
			Name: "not assigned, in the environment",
			Names: []string{
				"HOST",
			},
			Assignments:               []string{},
			EnvironmentVariablePrefix: "DEVPOD_PROVIDER_SSH_",
			NotInEnvironment:          []string{},
			Environment: map[string]string{
				"DEVPOD_PROVIDER_SSH_HOST": "another-box",
			},
			ExpectedAssignments: []string{
				"HOST=another-box",
			},
		},
	}

	for _, testCase := range testCases {
		fmt.Println(testCase.Name)

		for _, k := range testCase.NotInEnvironment {
			err := os.Unsetenv(k)
			if err != nil {
				t.Fatalf("unexpected error %v in %s", err, testCase.Name)
			}
		}
		for k, v := range testCase.Environment {
			err := os.Setenv(k, v)
			if err != nil {
				t.Fatalf("unexpected error %v in %s", err, testCase.Name)
			}
		}

		result := PropagateFromEnvironment(testCase.Assignments, testCase.Names, testCase.EnvironmentVariablePrefix)

		assert.DeepEqual(t, result, testCase.ExpectedAssignments)
	}
}
