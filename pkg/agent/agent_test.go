package agent

import (
	"os/exec"
	"testing"
	"time"

	"github.com/loft-sh/log"
)

func TestDockerReachableRetry(t *testing.T) {
	if !commandExists("docker") {
		t.Skip("Docker not available, skipping test")
	}

	logger := log.GetInstance()

	rootRequired, err := dockerReachable("", nil, logger)
	if err != nil {
		t.Logf("Docker ps failed (expected on some systems): %v", err)
	}

	rootRequired, err = dockerReachable("non-existent-docker", nil, logger)
	if err == nil {
		t.Error("Expected error for non-existent docker command")
	}
	if rootRequired {
		t.Logf("Root required for non-existent docker command (expected): %v", err)
	}
}

func commandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

func TestDockerReachableWithEnv(t *testing.T) {
	if !commandExists("docker") {
		t.Skip("Docker not available, skipping test")
	}

	logger := log.GetInstance()
	envs := map[string]string{
		"DOCKER_HOST": "unix:///var/run/docker.sock",
	}

	start := time.Now()
	_, err := dockerReachable("", envs, logger)
	duration := time.Since(start)

	if duration > 8*time.Second {
		t.Errorf("dockerReachable took too long: %v", duration)
	}

	t.Logf("dockerReachable completed in %v with error: %v", duration, err)
}
