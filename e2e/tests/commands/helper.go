package commands

import (
	"context"

	"github.com/skevetter/devpod/e2e/framework"
)

func setupDockerProvider(binDir string) *framework.Framework {
	f := framework.NewDefaultFramework(binDir)
	_ = f.DevPodProviderDelete(context.Background(), "docker")
	_ = f.DevPodProviderAdd(context.Background(), "docker", "-o", "DOCKER_PATH=docker")
	err := f.DevPodProviderUse(context.Background(), "docker")
	framework.ExpectNoError(err)
	return f
}
