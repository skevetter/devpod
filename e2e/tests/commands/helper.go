package commands

import (
	"context"

	"github.com/skevetter/devpod/e2e/framework"
)

func setupDockerProvider(binDir string) *framework.Framework {
	f := framework.NewDefaultFramework(binDir)
	err := f.SetupDockerProvider(context.Background())
	framework.ExpectNoError(err)
	return f
}
