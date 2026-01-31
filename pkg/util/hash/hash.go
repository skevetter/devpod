package hash

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/docker/docker/pkg/longpath"
	"github.com/moby/patternmatcher"
	"github.com/pkg/errors"
)

const maxFilesToRead = 5000

var (
	errFileReadOverLimit = errors.New("read files over limit")
)

func DirectoryHash(srcPath string, excludePatterns, includeFiles []string) (string, error) {
	srcPath, err := validateAndPreparePath(srcPath)
	if err != nil {
		return "", err
	}

	pm, err := patternmatcher.New(excludePatterns)
	if err != nil {
		return "", err
	}

	retFiles, err := collectFiles(srcPath, pm, includeFiles)
	if err != nil {
		return "", err
	}

	return computeFinalHash(retFiles)
}

func validateAndPreparePath(srcPath string) (string, error) {
	srcPath, err := filepath.Abs(srcPath)
	if err != nil {
		return "", err
	}

	fileInfo, err := os.Stat(srcPath)
	if err != nil {
		return "", err
	}

	if !fileInfo.IsDir() {
		return "", nil
	}

	if runtime.GOOS == "windows" {
		srcPath = longpath.AddPrefix(srcPath)
	}

	stat, err := os.Lstat(srcPath)
	if err != nil {
		return "", err
	}

	if !stat.IsDir() {
		return "", errors.Errorf("Path %s is not a directory", srcPath)
	}

	return srcPath, nil
}

func collectFiles(srcPath string, pm *patternmatcher.PatternMatcher, includeFiles []string) ([]string, error) {
	seen := make(map[string]bool)
	retFiles := []string{}
	walkRoot := filepath.Join(srcPath, ".")

	walker := &fileWalker{
		srcPath:      srcPath,
		pm:           pm,
		includeFiles: includeFiles,
		seen:         seen,
		retFiles:     &retFiles,
	}

	err := filepath.Walk(walkRoot, walker.walkFunc)
	if err != nil && !errors.Is(err, errFileReadOverLimit) {
		return nil, errors.Errorf("Error hashing %s: %v", srcPath, err)
	}

	return retFiles, nil
}

type fileWalker struct {
	srcPath      string
	pm           *patternmatcher.PatternMatcher
	includeFiles []string
	seen         map[string]bool
	retFiles     *[]string
}

func (w *fileWalker) walkFunc(filePath string, f os.FileInfo, err error) error {
	if err != nil {
		return errors.Errorf("Hash: Can't stat file %s to hash: %s", w.srcPath, err)
	}

	if len(*w.retFiles) >= maxFilesToRead {
		return errFileReadOverLimit
	}

	relFilePath, err := w.getRelativePath(filePath)
	if err != nil {
		return err
	}

	if !w.shouldIncludeFile(relFilePath) {
		return nil
	}

	if err := w.handleSkipLogic(relFilePath, f); err != nil {
		return err
	}

	if w.seen[relFilePath] {
		return nil
	}

	return w.processFile(filePath, relFilePath, f)
}

func (w *fileWalker) getRelativePath(filePath string) (string, error) {
	relFilePath, err := filepath.Rel(w.srcPath, filePath)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(relFilePath), nil
}

func (w *fileWalker) handleSkipLogic(relFilePath string, f os.FileInfo) error {
	skip, err := w.shouldSkipPath(relFilePath)
	if err != nil {
		return err
	}
	if skip {
		return w.handleDirectorySkip(relFilePath, f)
	}
	return nil
}

func (w *fileWalker) shouldIncludeFile(relFilePath string) bool {
	for _, f := range w.includeFiles {
		if strings.HasPrefix(relFilePath, f) {
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
		return false, errors.Errorf("Error matching %s: %v", relFilePath, err)
	}

	return skip, nil
}

func (w *fileWalker) handleDirectorySkip(relFilePath string, f os.FileInfo) error {
	if !f.IsDir() {
		return nil
	}

	if !w.pm.Exclusions() {
		return filepath.SkipDir
	}

	dirSlash := relFilePath + string(filepath.Separator)
	for _, pat := range w.pm.Patterns() {
		if !pat.Exclusion() {
			continue
		}

		if strings.HasPrefix(pat.String()+string(filepath.Separator), dirSlash) {
			return nil
		}
	}

	return filepath.SkipDir
}

func (w *fileWalker) processFile(filePath, relFilePath string, f os.FileInfo) error {
	w.seen[relFilePath] = true
	if !f.IsDir() {
		checksum, err := hashFileCRC32(filePath, 0xedb88320)
		if err != nil {
			return nil
		}

		*w.retFiles = append(*w.retFiles, relFilePath+";"+checksum)
	}

	return nil
}

func computeFinalHash(retFiles []string) (string, error) {
	if len(retFiles) == 0 {
		return "", nil
	}

	hash := sha256.New()
	sort.Strings(retFiles)
	for _, f := range retFiles {
		_, _ = hash.Write([]byte(f))
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func hashFileCRC32(filePath string, polynomial uint32) (string, error) {
	//Initialize an empty return string now in case an error has to be returned
	var returnCRC32String string

	//Open the fhe file located at the given path and check for errors
	file, err := os.Open(filePath)
	if err != nil {
		return returnCRC32String, err
	}

	//Tell the program to close the file when the function returns
	defer func() { _ = file.Close() }()

	//Create the table with the given polynomial
	tablePolynomial := crc32.MakeTable(polynomial)

	//Open a new hash interface to write the file to
	hash := crc32.New(tablePolynomial)

	//Copy the file in the interface
	if _, err := io.Copy(hash, file); err != nil {
		return returnCRC32String, err
	}

	//Generate the hash
	hashInBytes := hash.Sum(nil)[:]

	//Encode the hash to a string
	returnCRC32String = hex.EncodeToString(hashInBytes)

	//Return the output
	return returnCRC32String, nil
}
