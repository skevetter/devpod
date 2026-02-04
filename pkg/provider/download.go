package provider

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/skevetter/devpod/pkg/config"
	"github.com/skevetter/devpod/pkg/copy"
	"github.com/skevetter/devpod/pkg/download"
	"github.com/skevetter/devpod/pkg/extract"
	"github.com/skevetter/log"
	"github.com/skevetter/log/hash"
	"k8s.io/client-go/util/retry"
)

const (
	dirPerms    = 0750
	filePerms   = 0755
	windowsOS   = "windows"
	exeSuffix   = ".exe"
	httpPrefix  = "http://"
	httpsPrefix = "https://"
	gzSuffix    = ".gz"
	tarSuffix   = ".tar"
	tgzSuffix   = ".tgz"
	zipSuffix   = ".zip"
	cacheDir    = "devpod-binaries"
)

var (
	downloadBackoff               = retry.DefaultBackoff
	errChecksumVerificationFailed = errors.New("checksum verification failed")
)

type EnvironmentOptions struct {
	Context   string
	Workspace *Workspace
	Machine   *Machine
	Options   map[string]config.OptionValue
	Config    *ProviderConfig
	ExtraEnv  map[string]string
	Log       log.Logger
}

func ToEnvironmentWithBinaries(opts EnvironmentOptions) ([]string, error) {
	environ := ToEnvironment(opts.Workspace, opts.Machine, opts.Options, opts.ExtraEnv)
	binariesMap, err := GetBinaries(opts.Context, opts.Config)
	if err != nil {
		return nil, err
	}

	for k, v := range binariesMap {
		environ = append(environ, k+"="+v)
	}
	return environ, nil
}

func GetBinariesFrom(config *ProviderConfig, binariesDir string) (map[string]string, error) {
	retBinaries := map[string]string{}
	for binaryName, binaryLocations := range config.Binaries {
		found := false
		for _, binary := range binaryLocations {
			if binary.OS != runtime.GOOS || binary.Arch != runtime.GOARCH {
				continue
			}

			targetFolder := filepath.Join(binariesDir, strings.ToLower(binaryName))
			binaryPath := getBinaryPath(binary, targetFolder)
			if _, err := os.Stat(binaryPath); err != nil {
				return nil, fmt.Errorf("error trying to find binary %s: %w", binaryName, err)
			}

			retBinaries[binaryName] = binaryPath
			found = true
			break
		}
		if !found {
			return nil, fmt.Errorf(
				"cannot find provider binary %s, because no binary location matched OS %s and ARCH %s",
				binaryName, runtime.GOOS, runtime.GOARCH)
		}
	}

	return retBinaries, nil
}

func GetBinaries(context string, config *ProviderConfig) (map[string]string, error) {
	binariesDir, err := GetProviderBinariesDir(context, config.Name)
	if err != nil {
		return nil, err
	}

	return GetBinariesFrom(config, binariesDir)
}

func DownloadBinaries(
	binaries map[string][]*ProviderBinary,
	targetFolder string,
	log log.Logger,
) (map[string]string, error) {
	retBinaries := map[string]string{}
	for binaryName, binaryLocations := range binaries {
		binaryPath, err := downloadBinaryForPlatform(binaryName, binaryLocations, targetFolder, log)
		if err != nil {
			return nil, err
		}
		retBinaries[binaryName] = binaryPath
	}

	return retBinaries, nil
}

func downloadBinaryForPlatform(
	binaryName string,
	binaryLocations []*ProviderBinary,
	targetFolder string,
	log log.Logger,
) (string, error) {
	for _, binary := range binaryLocations {
		if binary.OS != runtime.GOOS || binary.Arch != runtime.GOARCH {
			continue
		}

		// check if binary is correct
		binaryTargetFolder := filepath.Join(targetFolder, strings.ToLower(binaryName))
		binaryPath := getBinaryPath(binary, binaryTargetFolder)
		if verifyOrRemoveBinary(binaryPath, binary.Checksum) ||
			fromCache(binary, binaryTargetFolder, log) {
			return binaryPath, nil
		}

		// try to download the binary
		binaryPath, err := downloadWithRetry(binaryName, binary, binaryTargetFolder, log)
		if err != nil {
			return "", err
		}
		return binaryPath, nil
	}
	return "", fmt.Errorf(
		"cannot download provider binary %s, because no binary location matched OS %s and ARCH %s",
		binaryName, runtime.GOOS, runtime.GOARCH)
}

func downloadWithRetry(
	binaryName string,
	binary *ProviderBinary,
	targetFolder string,
	log log.Logger,
) (string, error) {
	var binaryPath string
	err := retry.OnError(downloadBackoff, isRetriableError, func() error {
		path, err := downloadBinary(binaryName, binary, targetFolder, log)
		if err != nil {
			return err
		}

		if binary.Checksum != "" {
			if !verifyDownloadedBinary(path, binary, binaryName, log) {
				return errChecksumVerificationFailed
			}
		}

		binaryPath = path
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to download binary %s: %w", binaryName, err)
	}

	toCache(binary, binaryPath, log)
	return binaryPath, nil
}

func isRetriableError(err error) bool {
	if err == nil {
		return false
	}

	// Skip retry on checksum verification failures
	if errors.Is(err, errChecksumVerificationFailed) {
		return false
	}

	// Check for HTTP status codes using typed error
	var httpErr *download.HTTPStatusError
	if errors.As(err, &httpErr) {
		// Skip retry on 4xx client errors
		// (except 408 Request Timeout and 429 Too Many Requests)
		if httpErr.StatusCode >= http.StatusBadRequest &&
			httpErr.StatusCode < http.StatusInternalServerError &&
			httpErr.StatusCode != http.StatusRequestTimeout &&
			httpErr.StatusCode != http.StatusTooManyRequests {
			return false
		}
	}

	// Retry on network errors, timeouts, and 5xx errors
	return true
}

func verifyDownloadedBinary(
	binaryPath string,
	binary *ProviderBinary,
	binaryName string,
	log log.Logger,
) bool {
	fileHash, err := hash.File(binaryPath)
	if err != nil {
		_ = os.Remove(binaryPath)
		log.Errorf("error hashing %s: %v", binaryPath, err)
		return false
	}
	if !strings.EqualFold(fileHash, binary.Checksum) {
		_ = os.Remove(binaryPath)
		log.Errorf("unexpected file checksum %s != %s for binary %s",
			strings.ToLower(fileHash), strings.ToLower(binary.Checksum), binaryName)
		return false
	}
	return true
}

func toCache(binary *ProviderBinary, binaryPath string, log log.Logger) {
	if !isRemotePath(binary.Path) {
		return
	}

	cachedBinaryPath := getCachedBinaryPath(binary.Path)
	if err := os.MkdirAll(filepath.Dir(cachedBinaryPath), dirPerms); err != nil {
		log.Warnf("error creating cache directory: %v", err)
		return
	}

	if err := copy.File(binaryPath, cachedBinaryPath, filePerms); err != nil {
		log.Warnf("error copying binary to cache: %v", err)
	}
}

func fromCache(binary *ProviderBinary, targetFolder string, log log.Logger) bool {
	if !isRemotePath(binary.Path) {
		return false
	}

	binaryPath := getBinaryPath(binary, targetFolder)
	cachedBinaryPath := getCachedBinaryPath(binary.Path)
	if !verifyOrRemoveBinary(cachedBinaryPath, binary.Checksum) {
		return false
	}

	if err := os.MkdirAll(filepath.Dir(binaryPath), dirPerms); err != nil {
		log.Warnf("error creating directory %s: %v", filepath.Dir(binaryPath), err)
		return false
	}

	if err := copy.File(cachedBinaryPath, binaryPath, filePerms); err != nil {
		log.Warnf("error copying cached binary from %s to %s: %v", cachedBinaryPath, binaryPath, err)
		return false
	}

	return true
}

func getCachedBinaryPath(url string) string {
	return filepath.Join(os.TempDir(), cacheDir, hash.String(url)[:32])
}

func verifyOrRemoveBinary(binaryPath, checksum string) bool {
	_, err := os.Stat(binaryPath)
	if err != nil {
		return false
	}

	// verify checksum
	if checksum != "" {
		fileHash, err := hash.File(binaryPath)
		if err != nil || !strings.EqualFold(fileHash, checksum) {
			_ = os.Remove(binaryPath)
			return false
		}
	}

	return true
}

func getBinaryPath(binary *ProviderBinary, targetFolder string) string {
	if filepath.IsAbs(binary.Path) {
		return binary.Path
	}

	if !isRemotePath(binary.Path) {
		return localTargetPath(binary, targetFolder)
	}

	if binary.ArchivePath != "" {
		return filepath.Join(targetFolder, binary.ArchivePath)
	}

	name := binary.Name
	if name == "" {
		name = path.Base(binary.Path)
		if runtime.GOOS == windowsOS && !strings.HasSuffix(name, exeSuffix) {
			name += exeSuffix
		}
	}
	return filepath.Join(targetFolder, name)
}

func isRemotePath(p string) bool {
	return strings.HasPrefix(p, httpPrefix) || strings.HasPrefix(p, httpsPrefix)
}

func downloadBinary(
	binaryName string,
	binary *ProviderBinary,
	targetFolder string,
	log log.Logger,
) (string, error) {
	if _, err := os.Stat(binary.Path); err == nil {
		return handleLocalBinary(binary, targetFolder)
	}

	if !isRemotePath(binary.Path) {
		return handleNonHTTPBinary(binary, targetFolder)
	}

	if err := os.MkdirAll(targetFolder, dirPerms); err != nil {
		return "", fmt.Errorf("create folder: %w", err)
	}

	return downloadRemoteBinary(binaryName, binary, targetFolder, log)
}

func handleLocalBinary(binary *ProviderBinary, targetFolder string) (string, error) {
	if filepath.IsAbs(binary.Path) {
		return binary.Path, nil
	}

	if err := os.MkdirAll(targetFolder, dirPerms); err != nil {
		return "", fmt.Errorf("create folder: %w", err)
	}

	targetPath := localTargetPath(binary, targetFolder)
	if err := copyLocal(binary, targetPath); err != nil {
		_ = os.Remove(targetPath)
		return "", err
	}

	return targetPath, nil
}

func handleNonHTTPBinary(binary *ProviderBinary, targetFolder string) (string, error) {
	targetPath := localTargetPath(binary, targetFolder)
	if _, err := os.Stat(targetPath); err == nil {
		return targetPath, nil
	}
	return "", fmt.Errorf("cannot download %s as scheme is missing", binary.Path)
}

func downloadRemoteBinary(
	binaryName string,
	binary *ProviderBinary,
	targetFolder string,
	log log.Logger,
) (string, error) {
	var targetPath string
	var err error

	if binary.ArchivePath != "" {
		targetPath, err = downloadArchive(binaryName, binary, targetFolder, log)
	} else {
		targetPath, err = downloadFile(binaryName, binary, targetFolder, log)
	}

	if err != nil {
		_ = os.Remove(targetPath)
		return "", err
	}

	if err := os.Chmod(targetPath, filePerms); err != nil {
		return "", err
	}

	return targetPath, nil
}

func downloadFile(
	binaryName string,
	binary *ProviderBinary,
	targetFolder string,
	log log.Logger,
) (string, error) {
	name := binary.Name
	if name == "" {
		name = path.Base(binary.Path)
		if runtime.GOOS == windowsOS && !strings.HasSuffix(name, exeSuffix) {
			name += exeSuffix
		}
	}
	targetPath := filepath.Join(targetFolder, name)
	_, err := os.Stat(targetPath)
	if err == nil {
		return targetPath, nil
	}

	return downloadAndSaveFile(binaryName, binary, targetPath, log)
}

func downloadAndSaveFile(
	binaryName string,
	binary *ProviderBinary,
	targetPath string,
	log log.Logger,
) (string, error) {
	log.Infof("downloading binary %s from %s", binaryName, binary.Path)

	body, err := download.File(binary.Path, log)
	if err != nil {
		return "", fmt.Errorf("download binary: %w", err)
	}
	defer func() { _ = body.Close() }()

	// #nosec G304 - targetPath is constructed from validated binary configuration
	file, err := os.Create(targetPath)
	if err != nil {
		return targetPath, err
	}
	defer func() { _ = file.Close() }()

	if _, err := io.Copy(file, body); err != nil {
		return targetPath, fmt.Errorf("download file: %w", err)
	}

	log.Debugf("downloaded binary %s", binaryName)
	return targetPath, nil
}

func downloadArchive(
	binaryName string,
	binary *ProviderBinary,
	targetFolder string,
	log log.Logger,
) (string, error) {
	targetPath := filepath.Join(targetFolder, binary.ArchivePath)
	_, err := os.Stat(targetPath)
	if err == nil {
		return targetPath, nil
	}

	return extractArchive(archiveDownloadParams{
		binaryName:   binaryName,
		binary:       binary,
		targetFolder: targetFolder,
		targetPath:   targetPath,
		log:          log,
	})
}

type archiveDownloadParams struct {
	binaryName   string
	binary       *ProviderBinary
	targetFolder string
	targetPath   string
	log          log.Logger
}

func extractArchive(params archiveDownloadParams) (string, error) {
	params.log.Infof("downloading binary %s from %s", params.binaryName, params.binary.Path)

	body, err := download.File(params.binary.Path, params.log)
	if err != nil {
		return "", err
	}
	defer func() { _ = body.Close() }()

	targetPath, err := extractByFormat(body, params)
	if err != nil {
		return "", err
	}

	params.log.Debugf("extracted and downloaded archive %s", params.binaryName)
	return targetPath, nil
}

func extractByFormat(body io.ReadCloser, params archiveDownloadParams) (string, error) {
	if isGzipOrTar(params.binary.Path) {
		if err := extract.Extract(body, params.targetFolder); err != nil {
			return "", err
		}
		return params.targetPath, nil
	}

	if strings.HasSuffix(params.binary.Path, zipSuffix) {
		return extractZipArchive(body, params.targetFolder, params.targetPath)
	}

	return "", fmt.Errorf("unrecognized archive format %s", params.binary.Path)
}

func isGzipOrTar(path string) bool {
	return strings.HasSuffix(path, gzSuffix) ||
		strings.HasSuffix(path, tarSuffix) ||
		strings.HasSuffix(path, tgzSuffix)
}

func extractZipArchive(body io.ReadCloser, targetFolder, targetPath string) (string, error) {
	tempFile, err := downloadToTempFile(body)
	if err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(tempFile) }()

	if err := extract.UnzipFolder(tempFile, targetFolder); err != nil {
		return "", err
	}

	return targetPath, nil
}

func downloadToTempFile(reader io.Reader) (string, error) {
	tempFile, err := os.CreateTemp("", "")
	if err != nil {
		return "", err
	}
	defer func() { _ = tempFile.Close() }()

	if _, err := io.Copy(tempFile, reader); err != nil {
		_ = os.Remove(tempFile.Name())
		return "", err
	}

	return tempFile.Name(), nil
}

func localTargetPath(binary *ProviderBinary, targetFolder string) string {
	name := binary.Name
	if name == "" {
		name = path.Base(binary.Path)
	}
	return filepath.Join(targetFolder, name)
}

func copyLocal(binary *ProviderBinary, targetPath string) error {
	targetPathStat, err := os.Stat(targetPath)
	if err == nil {
		binaryStat, err := os.Stat(binary.Path)
		if err != nil {
			return err
		}
		if targetPathStat.Size() == binaryStat.Size() &&
			!binaryStat.ModTime().After(targetPathStat.ModTime()) {
			return nil
		}
	}

	return copy.File(binary.Path, targetPath, filePerms)
}
