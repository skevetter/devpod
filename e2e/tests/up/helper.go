package up

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
	docker "github.com/skevetter/devpod/pkg/docker"
	"github.com/skevetter/log"
	"github.com/skevetter/log/scanner"
)

type baseTestContext struct {
	f            *framework.Framework
	dockerHelper *docker.DockerHelper
	initialDir   string
}

func (btc *baseTestContext) execSSHCapture(ctx context.Context, projectName, command string) (string, error) {
	output, _, err := btc.f.ExecCommandCapture(ctx, []string{"ssh", "--command", command, projectName})
	return output, err
}

func (btc *baseTestContext) execSSH(ctx context.Context, tempDir, command string) (string, error) {
	return btc.f.DevPodSSH(ctx, tempDir, command)
}

// Log scanning functions
func findMessage(reader io.Reader, message string) error {
	scan := scanner.NewScanner(reader)
	for scan.Scan() {
		if line := scan.Bytes(); len(line) > 0 {
			lineObject := &log.Line{}
			if err := json.Unmarshal(line, lineObject); err == nil && strings.Contains(lineObject.Message, message) {
				return nil
			}
		}
	}
	return fmt.Errorf("couldn't find message '%s' in log", message)
}

func verifyLogStream(reader io.Reader) error {
	scan := scanner.NewScanner(reader)
	for scan.Scan() {
		if line := scan.Bytes(); len(line) > 0 {
			lineObject := &log.Line{}
			if err := json.Unmarshal(line, lineObject); err != nil {
				return fmt.Errorf("error reading line %s %w", string(line), err)
			}
		}
	}
	return nil
}

func setupWorkspace(testdataPath, initialDir string, f *framework.Framework) (string, error) {
	tempDir, err := framework.CopyToTempDir(testdataPath)
	if err != nil {
		return "", err
	}
	ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)
	ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, tempDir)
	return tempDir, nil
}

func setupDockerProvider(binDir, dockerPath string) (*framework.Framework, error) {
	f := framework.NewDefaultFramework(binDir)
	_ = f.DevPodProviderDelete(context.Background(), "docker")
	_ = f.DevPodProviderAdd(context.Background(), "docker", "-o", "DOCKER_PATH="+dockerPath)
	return f, f.DevPodProviderUse(context.Background(), "docker")
}

func setupWorkspaceAndUp(ctx context.Context, testdataPath, initialDir string, f *framework.Framework, args ...string) (string, error) {
	tempDir, err := setupWorkspace(testdataPath, initialDir, f)
	if err != nil {
		return "", err
	}
	return tempDir, f.DevPodUp(ctx, append([]string{tempDir}, args...)...)
}
