package hash

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/docker/docker/pkg/longpath"
	"github.com/moby/patternmatcher"
	"golang.org/x/mod/sumdb/dirhash"
)

const maxFilesToRead = 5000

var errFileReadOverLimit = errors.New("read files over limit")

// DirectoryHash computes a hash of the directory contents using the standard dirhash.Hash1 algorithm.
// It supports filtering via exclude patterns (.dockerignore) and include filters (specific subdirectories).
// It processes up to maxFilesToRead files. If this limit is exceeded, it returns
// a warning error along with a partial hash computed from the first 5000 files.
//
// The hash format is "h1:" followed by base64-encoded SHA-256, compatible with go.sum format.
//
// Parameters:
//   - srcPath: The directory path to hash
//   - excludePatterns: Patterns to exclude from hashing (e.g., .dockerignore patterns)
//   - includeFiles: Specific files to include in the hash
//
// Returns:
//   - hash: "h1:" format hash of the directory contents (may be partial if limit exceeded)
//   - error: nil on success, warning error if file limit exceeded, or actual error on failure
func DirectoryHash(srcPath string, excludePatterns, includeFiles []string) (string, error) {
	srcPath, err := validateAndPreparePath(srcPath)
	if err != nil {
		return "", err
	}

	pm, err := patternmatcher.New(excludePatterns)
	if err != nil {
		return "", err
	}

	files, err := collectFiles(srcPath, pm, includeFiles)
	if err != nil && !errors.Is(err, errFileReadOverLimit) {
		return "", err
	}

	// Use dirhash.Hash1 for standard hashing
	hash, hashErr := dirhash.Hash1(files, func(name string) (io.ReadCloser, error) {
		// #nosec G304 - file path is constructed from validated srcPath and filtered file list
		return os.Open(filepath.Join(srcPath, filepath.FromSlash(name)))
	})

	if hashErr != nil {
		return "", hashErr
	}

	// If file limit was exceeded, return hash with warning
	if err != nil {
		return hash, err
	}

	return hash, nil
}

func validateAndPreparePath(srcPath string) (string, error) {
	srcPath, err := filepath.Abs(srcPath)
	if err != nil {
		return "", err
	}

	srcPath, err = filepath.EvalSymlinks(srcPath)
	if err != nil {
		return "", err
	}

	if runtime.GOOS == "windows" {
		srcPath = longpath.AddPrefix(srcPath)
	}

	fileInfo, err := os.Stat(srcPath)
	if err != nil {
		return "", err
	}

	if !fileInfo.IsDir() {
		return "", fmt.Errorf("path %s is not a directory", srcPath)
	}

	return srcPath, nil
}

func collectFiles(srcPath string, pm *patternmatcher.PatternMatcher, includeFiles []string) ([]string, error) {
	retFiles := []string{}

	// Normalize includeFiles once to forward slashes for consistent matching
	normalizedIncludes := make([]string, len(includeFiles))
	for i, f := range includeFiles {
		normalizedIncludes[i] = filepath.ToSlash(filepath.Clean(strings.TrimRight(f, "/\\")))
	}

	walker := &fileWalker{
		srcPath:      srcPath,
		pm:           pm,
		includeFiles: normalizedIncludes,
		retFiles:     &retFiles,
	}

	err := filepath.WalkDir(srcPath, walker.walkDirFunc)
	if err != nil {
		if errors.Is(err, errFileReadOverLimit) {
			return retFiles, fmt.Errorf(
				"directory hash incomplete: exceeded limit of %d files (partial hash computed from first %d files): %w",
				maxFilesToRead, len(retFiles), errFileReadOverLimit,
			)
		}
		return nil, fmt.Errorf("failed to hash %s: %w", srcPath, err)
	}

	return retFiles, nil
}

type fileWalker struct {
	srcPath      string
	pm           *patternmatcher.PatternMatcher
	includeFiles []string
	retFiles     *[]string
}

func (w *fileWalker) walkDirFunc(filePath string, d os.DirEntry, err error) error {
	if err != nil {
		return fmt.Errorf("error walking %s: %w", filePath, err)
	}

	if len(*w.retFiles) >= maxFilesToRead {
		return errFileReadOverLimit
	}

	relFilePath, err := w.getRelativePath(filePath)
	if err != nil {
		return err
	}

	return w.processEntry(relFilePath, d)
}

func (w *fileWalker) processEntry(relFilePath string, d os.DirEntry) error {
	if shouldSkip, skipErr := w.shouldSkip(relFilePath, d); skipErr != nil {
		return skipErr
	} else if shouldSkip {
		return nil
	}

	if !w.shouldIncludeFile(relFilePath) {
		return nil
	}

	if !d.IsDir() {
		*w.retFiles = append(*w.retFiles, relFilePath)
	}

	return nil
}

func (w *fileWalker) shouldSkip(relFilePath string, d os.DirEntry) (bool, error) {
	skip, err := w.shouldSkipPath(relFilePath)
	if err != nil {
		return false, err
	}
	if skip {
		if d.IsDir() {
			return true, w.handleDirectorySkip(relFilePath)
		}
		return true, nil
	}
	return false, nil
}

func (w *fileWalker) getRelativePath(filePath string) (string, error) {
	relFilePath, err := filepath.Rel(w.srcPath, filePath)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(relFilePath), nil
}

func (w *fileWalker) shouldIncludeFile(relFilePath string) bool {
	if len(w.includeFiles) == 0 {
		return true
	}

	relFilePath = filepath.Clean(relFilePath)

	for _, f := range w.includeFiles {
		// includeFiles are already normalized to forward slashes in collectFiles
		if f == relFilePath || strings.HasPrefix(relFilePath, f+"/") {
			return true
		}
	}
	return false
}

func (w *fileWalker) shouldSkipPath(relFilePath string) (bool, error) {
	if relFilePath == "." {
		return false, nil
	}

	skip, err := w.pm.MatchesOrParentMatches(relFilePath)
	if err != nil {
		return false, fmt.Errorf("failed to match %s: %v", relFilePath, err)
	}

	return skip, nil
}

func (w *fileWalker) handleDirectorySkip(relFilePath string) error {
	if !w.pm.Exclusions() {
		return filepath.SkipDir
	}

	// Use forward slash consistently since relFilePath is normalized with filepath.ToSlash
	dirSlash := relFilePath + "/"
	for _, pat := range w.pm.Patterns() {
		if !pat.Exclusion() {
			continue
		}

		if strings.HasPrefix(pat.String()+"/", dirSlash) {
			return nil
		}
	}

	return filepath.SkipDir
}
