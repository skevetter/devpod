package up

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
	"github.com/skevetter/devpod/pkg/compose"
	docker "github.com/skevetter/devpod/pkg/docker"
	provider2 "github.com/skevetter/devpod/pkg/provider"
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

func (btc *baseTestContext) inspectContainer(ctx context.Context, ids []string) (*container.InspectResponse, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("no container IDs provided")
	}
	var details []container.InspectResponse
	if err := btc.dockerHelper.Inspect(ctx, ids, "container", &details); err != nil {
		return nil, err
	}
	return &details[0], nil
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

func createTarGzArchive(outputFilePath string, filePaths []string) error {
	outputFile, err := os.Create(outputFilePath)
	if err != nil {
		return err
	}
	defer func() { _ = outputFile.Close() }()

	gzipWriter := gzip.NewWriter(outputFile)
	defer func() { _ = gzipWriter.Close() }()

	tarWriter := tar.NewWriter(gzipWriter)
	defer func() { _ = tarWriter.Close() }()

	for _, filePath := range filePaths {
		if err := addFileToTar(tarWriter, filePath); err != nil {
			return err
		}
	}
	return nil
}

func addFileToTar(tarWriter *tar.Writer, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	fileInfo, err := file.Stat()
	if err != nil {
		return err
	}

	header, err := tar.FileInfoHeader(fileInfo, fileInfo.Name())
	if err != nil {
		return err
	}

	if err := tarWriter.WriteHeader(header); err != nil {
		return err
	}

	_, err = io.Copy(tarWriter, file)
	return err
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

func findComposeContainer(ctx context.Context, dockerHelper *docker.DockerHelper, composeHelper *compose.ComposeHelper, workspaceUID, serviceName string) ([]string, error) {
	return dockerHelper.FindContainer(ctx, []string{
		fmt.Sprintf("%s=%s", compose.ProjectLabel, composeHelper.GetProjectName(workspaceUID)),
		fmt.Sprintf("%s=%s", compose.ServiceLabel, serviceName),
	})
}

func devPodUpAndFindWorkspace(ctx context.Context, f *framework.Framework, tempDir string, args ...string) (*provider2.Workspace, error) {
	if err := f.DevPodUp(ctx, append([]string{tempDir}, args...)...); err != nil {
		return nil, err
	}
	return f.FindWorkspace(ctx, tempDir)
}
