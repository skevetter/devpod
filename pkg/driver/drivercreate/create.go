package drivercreate

import (
	"fmt"

	"github.com/loft-sh/log"
	"github.com/skevetter/devpod/pkg/driver"
	"github.com/skevetter/devpod/pkg/driver/custom"
	"github.com/skevetter/devpod/pkg/driver/docker"
	"github.com/skevetter/devpod/pkg/driver/kubernetes"
	provider2 "github.com/skevetter/devpod/pkg/provider"
)

func NewDriver(workspaceInfo *provider2.AgentWorkspaceInfo, log log.Logger) (driver.Driver, error) {
	driver := workspaceInfo.Agent.Driver
	switch driver {
	case "", provider2.DockerDriver:
		return docker.NewDockerDriver(workspaceInfo, log)
	case provider2.CustomDriver:
		return custom.NewCustomDriver(workspaceInfo, log), nil
	case provider2.KubernetesDriver:
		return kubernetes.NewKubernetesDriver(workspaceInfo, log)
	}

	return nil, fmt.Errorf("unrecognized driver '%s', possible values are %s, %s or %s",
		driver, provider2.DockerDriver, provider2.CustomDriver, provider2.KubernetesDriver)
}
