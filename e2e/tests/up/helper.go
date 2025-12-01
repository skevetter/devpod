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

	"github.com/loft-sh/log"
	"github.com/loft-sh/log/scanner"
	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
	"github.com/skevetter/devpod/pkg/compose"
	docker "github.com/skevetter/devpod/pkg/docker"
	provider2 "github.com/skevetter/devpod/pkg/provider"
)

func findMessage(reader io.Reader, message string) error {
	scan := scanner.NewScanner(reader)
	for scan.Scan() {
		line := scan.Bytes()
		if len(line) == 0 {
			continue
		}

		lineObject := &log.Line{}
		err := json.Unmarshal(line, lineObject)
		if err == nil && strings.Contains(lineObject.Message, message) {
			return nil
		}
	}

	return fmt.Errorf("couldn't find message '%s' in log", message)
}

func verifyLogStream(reader io.Reader) error {
	scan := scanner.NewScanner(reader)
	for scan.Scan() {
		line := scan.Bytes()
		if len(line) == 0 {
			continue
		}

		lineObject := &log.Line{}
		err := json.Unmarshal(line, lineObject)
		if err != nil {
			return fmt.Errorf("error reading line %s: %w", string(line), err)
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
	defer func() { _ = gzipWriter.Close() }()

	for _, filePath := range filePaths {
		file, err := os.Open(filePath)
		if err != nil {
			return err
		}
		defer func() { _ = file.Close() }()

		fileInfo, err := file.Stat()
		if err != nil {
			return err
		}

		fileInfoHdr, err := tar.FileInfoHeader(fileInfo, fileInfo.Name())
		if err != nil {
			return err
		}

		err = tarWriter.WriteHeader(fileInfoHdr)
		if err != nil {
			return err
		}

		_, err = io.Copy(tarWriter, file)
		if err != nil {
			return err
		}
	}
	return nil
}

// setupWorkspace copies testdata to a temp directory and registers cleanup
func setupWorkspace(testdataPath, initialDir string, f *framework.Framework) (string, error) {
	tempDir, err := framework.CopyToTempDir(testdataPath)
	if err != nil {
		return "", err
	}
	ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)
	ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, tempDir)
	return tempDir, nil
}

// setupDockerProvider creates a framework and configures the docker provider with the specified docker path
func setupDockerProvider(binDir, dockerPath string) (*framework.Framework, error) {
	f := framework.NewDefaultFramework(binDir)
	_ = f.DevPodProviderAdd(context.Background(), "docker", "-o", "DOCKER_PATH="+dockerPath)
	err := f.DevPodProviderUse(context.Background(), "docker")
	return f, err
}

// findComposeContainer finds a compose container by project and service name
func findComposeContainer(ctx context.Context, dockerHelper *docker.DockerHelper, composeHelper *compose.ComposeHelper, workspaceUID, serviceName string) ([]string, error) {
	return dockerHelper.FindContainer(ctx, []string{
		fmt.Sprintf("%s=%s", compose.ProjectLabel, composeHelper.GetProjectName(workspaceUID)),
		fmt.Sprintf("%s=%s", compose.ServiceLabel, serviceName),
	})
}

// devPodUpAndFindWorkspace runs devpod up and returns the workspace
func devPodUpAndFindWorkspace(ctx context.Context, f *framework.Framework, tempDir string, args ...string) (*provider2.Workspace, error) {
	allArgs := append([]string{tempDir}, args...)
	err := f.DevPodUp(ctx, allArgs...)
	if err != nil {
		return nil, err
	}
	return f.FindWorkspace(ctx, tempDir)
}
