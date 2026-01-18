package setup

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type MergeRemoteEnvTestSuite struct {
	suite.Suite
}

func TestMergeRemoteEnvTestSuite(t *testing.T) {
	suite.Run(t, new(MergeRemoteEnvTestSuite))
}

func (s *MergeRemoteEnvTestSuite) TestPathFromRemoteEnvUsedExactly() {
	remoteEnv := map[string]string{
		"PATH":   "/remoteEnv/marker:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"MARKER": "remote",
	}
	probedEnv := map[string]string{
		"PATH":   "/home/user/.local/bin:/usr/local/bin:/usr/bin:/bin",
		"MARKER": "probed",
	}

	result := mergeRemoteEnv(remoteEnv, probedEnv)

	s.Equal("/remoteEnv/marker:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin", result["PATH"])
	s.Equal("remote", result["MARKER"])
}

func (s *MergeRemoteEnvTestSuite) TestCustomPathsAppearFirst() {
	remoteEnv := map[string]string{
		"PATH": "/custom/path:/another/custom:/usr/bin:/bin",
	}
	probedEnv := map[string]string{
		"PATH": "/usr/local/bin:/usr/bin:/bin",
	}

	result := mergeRemoteEnv(remoteEnv, probedEnv)

	s.Equal("/custom/path:/another/custom:/usr/bin:/bin", result["PATH"])
}

func (s *MergeRemoteEnvTestSuite) TestOtherEnvVariablesPassThrough() {
	remoteEnv := map[string]string{
		"PATH":        "/custom:/usr/bin",
		"CUSTOM_VAR":  "custom_value",
		"ANOTHER_VAR": "another_value",
	}
	probedEnv := map[string]string{
		"PATH":       "/usr/bin:/bin",
		"PROBED_VAR": "probed_value",
	}

	result := mergeRemoteEnv(remoteEnv, probedEnv)

	s.Equal("/custom:/usr/bin", result["PATH"])
	s.Equal("custom_value", result["CUSTOM_VAR"])
	s.Equal("another_value", result["ANOTHER_VAR"])
	s.Equal("probed_value", result["PROBED_VAR"])
}

func (s *MergeRemoteEnvTestSuite) TestUseProbedPathWhenNotInRemoteEnv() {
	remoteEnv := map[string]string{
		"MARKER": "remote",
	}
	probedEnv := map[string]string{
		"PATH":   "/usr/local/bin:/usr/bin:/bin",
		"MARKER": "probed",
	}

	result := mergeRemoteEnv(remoteEnv, probedEnv)

	s.Equal("/usr/local/bin:/usr/bin:/bin", result["PATH"])
	s.Equal("remote", result["MARKER"])
}

func (s *MergeRemoteEnvTestSuite) TestRemoteEnvOverridesProbedEnv() {
	remoteEnv := map[string]string{
		"PATH":   "/custom:/usr/bin",
		"MARKER": "remote",
	}
	probedEnv := map[string]string{
		"PATH":   "/usr/bin:/bin",
		"MARKER": "probed",
	}

	result := mergeRemoteEnv(remoteEnv, probedEnv)

	s.Equal("/custom:/usr/bin", result["PATH"])
	s.Equal("remote", result["MARKER"])
}

func (s *MergeRemoteEnvTestSuite) TestPathOrderPreserved() {
	remoteEnv := map[string]string{
		"PATH": "/first:/second:/third:/usr/bin:/bin",
	}
	probedEnv := map[string]string{
		"PATH": "/home/user/.local/bin:/usr/local/bin:/usr/bin:/bin",
	}

	result := mergeRemoteEnv(remoteEnv, probedEnv)

	s.Equal("/first:/second:/third:/usr/bin:/bin", result["PATH"])
	s.True(result["PATH"][:6] == "/first", "Custom path /first should be first in PATH")
}
