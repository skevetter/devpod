package framework

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func GetTimeout() time.Duration {
	if runtime.GOOS == "windows" {
		return 600 * time.Second
	}

	// NOTE: Downloading container images can be slow depending on network conditions.
	// If tests need to download a large container image or multiple images, it
	// may fail due to timeout. Recommend, pre-staging images in local Docker cache
	// during the workflow before the start of the tests to mitigate timeout issues
	// caused by slow image downloads.
	return 120 * time.Second
}

func CreateTempDir() (string, error) {
	return createTempDir("")
}

func CopyToTempDirWithoutChdir(relativePath string) (string, error) {
	dir, err := createTempDir("")
	if err != nil {
		return "", err
	}

	absPath, err := filepath.Abs(relativePath)
	if err != nil {
		return "", err
	}

	err = copyDir(absPath, dir)
	if err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}

	return dir, nil
}

func CopyToTempDirInDir(baseDir, relativePath string) (string, error) {
	absPath, err := filepath.Abs(relativePath)
	if err != nil {
		return "", err
	}

	dir, err := createTempDir(baseDir)
	if err != nil {
		return "", err
	}

	err = os.Chdir(dir)
	if err != nil {
		return "", err
	}

	err = copyDir(absPath, dir)
	if err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}

	return dir, nil
}

func CopyToTempDir(relativePath string) (string, error) {
	return CopyToTempDirInDir("", relativePath)
}

func CleanupTempDir(initialDir, tempDir string) {
	err := os.Chdir(initialDir)
	ExpectNoError(err)

	err = os.RemoveAll(tempDir)
	if err != nil {
		fmt.Println("WARN:", err)
	}
}

func CleanString(input string) string {
	input = strings.ReplaceAll(input, "\\", "")
	return strings.ReplaceAll(input, "/", "")
}

// createTempDir creates a temporary directory based on environment and base directory
func createTempDir(baseDir string) (string, error) {
	var dir string
	var err error

	if os.Getenv("GITHUB_ACTIONS") == "true" {
		runnerTemp := os.Getenv("RUNNER_TEMP")
		if runnerTemp != "" {
			dir, err = os.MkdirTemp(runnerTemp, "temp-*")
		} else {
			dir, err = os.MkdirTemp("", "temp-*")
		}
	} else if os.Getenv("ACT") == "true" {
		dir, err = os.MkdirTemp("/tmp", "temp-*")
	} else {
		if baseDir == "" {
			dir, err = os.MkdirTemp("", "temp-*")
		} else {
			dir, err = os.MkdirTemp(baseDir, "temp-*")
		}
	}

	if err != nil {
		return "", err
	}

	// ensure temp dir path is an absolute path
	return filepath.EvalSymlinks(dir)
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, 0755)
		}

		return copyFile(path, dstPath)
	})
}

func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = srcFile.Close() }()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = dstFile.Close() }()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	return os.Chmod(dst, 0644)
}
