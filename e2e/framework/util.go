package framework

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/tabwriter"
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
	// Create temporary directory
	dir, err := os.MkdirTemp("", "temp-*")
	if err != nil {
		return "", err
	}

	// Make sure temp dir path is an absolute path
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}

	return dir, nil
}

func CopyToTempDirWithoutChdir(relativePath string) (string, error) {
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
		dir, err = os.MkdirTemp("", "temp-*")
	}

	if err != nil {
		return "", err
	}

	if os.Getenv("GITHUB_ACTIONS") == "true" {
		err = os.Chmod(dir, 0755)
		if err != nil {
			_ = os.RemoveAll(dir)
			return "", err
		}
	}

	// Make sure temp dir path is an absolute path
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}

	err = copyDir(relativePath, dir)
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

	var dir string

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
		dir, err = os.MkdirTemp(baseDir, "temp-*")
	}

	if err != nil {
		return "", err
	}

	if os.Getenv("GITHUB_ACTIONS") == "true" {
		err = os.Chmod(dir, 0755)
		if err != nil {
			_ = os.RemoveAll(dir)
			return "", err
		}
	}

	// Make sure temp dir path is an absolute path
	dir, err = filepath.EvalSymlinks(dir)
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

	fmt.Printf("=== DEBUG: After Copy ===\n")
	displayDirectoryInfo(dir)
	fmt.Printf("========================\n")

	return dir, nil
}

func displayDirectoryInfo(dir string) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	if stat, err := os.Stat(dir); err == nil {
		_, _ = fmt.Fprintf(w, "Directory:\t%s\n", dir)
		_, _ = fmt.Fprintf(w, "Mode:\t%v\n", stat.Mode())
		_, _ = fmt.Fprintf(w, "Size:\t%d bytes\n", stat.Size())
	}
	if files, err := os.ReadDir(dir); err == nil {
		_, _ = fmt.Fprintf(w, "Files:\t%d total\n", len(files))
		_, _ = fmt.Fprintln(w, "\t")
		if runtime.GOOS == "windows" {
			_, _ = fmt.Fprintf(w, "Name\tType\tMode\tSize\n")
			_, _ = fmt.Fprintf(w, "----\t----\t----\t----\n")
		} else {
			_, _ = fmt.Fprintf(w, "Name\tType\tMode\tSize\tUID\tGID\n")
			_, _ = fmt.Fprintf(w, "----\t----\t----\t----\t---\t---\n")
		}
		for _, file := range files {
			if info, err := file.Info(); err == nil {
				fileType := "file"
				if file.IsDir() {
					fileType = "dir"
				}
				if runtime.GOOS == "windows" {
					_, _ = fmt.Fprintf(w, "%s\t%s\t%v\t%d bytes\n", file.Name(), fileType, info.Mode(), info.Size())
				} else {
					uid, gid := getFileOwnership(info)
					_, _ = fmt.Fprintf(w, "%s\t%s\t%v\t%d bytes\t%s\t%s\n", file.Name(), fileType, info.Mode(), info.Size(), uid, gid)
				}
			}
		}
	}
	_ = w.Flush()
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

func CopyToTempDir(relativePath string) (string, error) {
	return CopyToTempDirInDir("", relativePath)
}

func CleanupTempDir(initialDir, tempDir string) {
	fmt.Printf("=== DEBUG: Before Cleanup ===\n")
	displayDirectoryInfo(tempDir)
	fmt.Printf("========================\n")

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
