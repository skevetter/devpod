package up

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"io"
	"os"

	"github.com/skevetter/devpod/e2e/framework"
)

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

func setupDockerProvider(binDir, dockerPath string) (*framework.Framework, error) {
	f := framework.NewDefaultFramework(binDir)
	_ = f.DevPodProviderDelete(context.Background(), "docker")
	_ = f.DevPodProviderAdd(context.Background(), "docker", "-o", "DOCKER_PATH="+dockerPath)
	return f, f.DevPodProviderUse(context.Background(), "docker")
}
