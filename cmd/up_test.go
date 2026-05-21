package cmd

import (
	"testing"

	"github.com/skevetter/devpod/cmd/flags"
	"github.com/stretchr/testify/require"
)

func TestUpMountFlagIsRepeatableAndPreservesValues(t *testing.T) {
	upCmd := NewUpCmd(&flags.GlobalFlags{})
	args := []string{
		"--mount", `{"type":"volume","source":"cache","target":"/cache"}`,
		"--mount", `type=bind,source=/tmp/data,target=/data,readonly`,
	}

	require.NoError(t, upCmd.ParseFlags(args))

	mounts, err := upCmd.Flags().GetStringArray("mount")
	require.NoError(t, err)
	require.Equal(t, []string{
		`{"type":"volume","source":"cache","target":"/cache"}`,
		`type=bind,source=/tmp/data,target=/data,readonly`,
	}, mounts)
}
