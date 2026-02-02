package binaries

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/skevetter/devpod/pkg/config"
	"github.com/skevetter/devpod/pkg/copy"
	"github.com/skevetter/devpod/pkg/download"
	"github.com/skevetter/devpod/pkg/extract"
	provider2 "github.com/skevetter/devpod/pkg/provider"
	"github.com/skevetter/log"
	"github.com/skevetter/log/hash"
)

const (
	retryCount  = 3
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

type EnvironmentOptions struct {
	Context   string
	Workspace *provider2.Workspace
	Machine   *provider2.Machine
	Options   map[string]config.OptionValue
	Config    *provider2.ProviderConfig
	ExtraEnv  map[string]string
	Log       log.Logger
}

func ToEnvironmentWithBinaries(opts EnvironmentOptions) ([]string, error) {
	environ := provider2.ToEnvironment(opts.Workspace, opts.Machine, opts.Options, opts.ExtraEnv)
	binariesMap, err := GetBinaries(opts.Context, opts.Config)
	if err != nil {
		return nil, err
	}

	for k, v := range binariesMap {
		environ = append(environ, k+"="+v)
	}
	return environ, nil
}

func GetBinariesFrom(config *provider2.ProviderConfig, binariesDir string) (map[string]string, error) {
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
				return nil, fmt.Errorf("error trying to find binary %s %w", binaryName, err)
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

func GetBinaries(context string, config *provider2.ProviderConfig) (map[string]string, error) {
	binariesDir, err := provider2.GetProviderBinariesDir(context, config.Name)
	if err != nil {
		return nil, err
	}

	return GetBinariesFrom(config, binariesDir)
}

func DownloadBinaries(
	binaries map[string][]*provider2.ProviderBinary,
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
	binaryLocations []*provider2.ProviderBinary,
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
		if verifyBinary(binaryPath, binary.Checksum) || fromCache(binary, binaryTargetFolder, log) {
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
	binary *provider2.ProviderBinary,
	targetFolder string,
	log log.Logger,
) (string, error) {
	var lastErr error
	for range retryCount {
		binaryPath, err := downloadBinary(binaryName, binary, targetFolder, log)
		if err != nil {
			lastErr = err
			continue
		}

		if binary.Checksum != "" {
			if !verifyDownloadedBinary(binaryPath, binary, binaryName, log) {
				lastErr = fmt.Errorf("checksum verification failed")
				continue
			}
		}

		toCache(binary, binaryPath, log)
		return binaryPath, nil
	}
	return "", fmt.Errorf("failed to download binary %s after %d attempts: %w", binaryName, retryCount, lastErr)
}

func verifyDownloadedBinary(
	binaryPath string,
	binary *provider2.ProviderBinary,
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

func toCache(binary *provider2.ProviderBinary, binaryPath string, log log.Logger) {
	if !isRemotePath(binary.Path) {
		return
	}

	cachedBinaryPath := getCachedBinaryPath(binary.Path)
	if err := os.MkdirAll(filepath.Dir(cachedBinaryPath), dirPerms); err != nil {
		return
	}

	if err := copy.File(binaryPath, cachedBinaryPath, filePerms); err != nil {
		log.Warnf("error copying binary to cache: %v", err)
	}
}

func fromCache(binary *provider2.ProviderBinary, targetFolder string, log log.Logger) bool {
	if !isRemotePath(binary.Path) {
		return false
	}

	binaryPath := getBinaryPath(binary, targetFolder)
	cachedBinaryPath := getCachedBinaryPath(binary.Path)
	if !verifyBinary(cachedBinaryPath, binary.Checksum) {
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
	return filepath.Join(os.TempDir(), cacheDir, hash.String(url)[:16])
}

func verifyBinary(binaryPath, checksum string) bool {
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

func getBinaryPath(binary *provider2.ProviderBinary, targetFolder string) string {
	if filepath.IsAbs(binary.Path) {
		return binary.Path
	}

	if !isRemotePath(binary.Path) {
		return localTargetPath(binary, targetFolder)
	}

	if binary.ArchivePath != "" {
		return path.Join(filepath.ToSlash(targetFolder), binary.ArchivePath)
	}

	name := binary.Name
	if name == "" {
		name = path.Base(binary.Path)
		if runtime.GOOS == windowsOS && !strings.HasSuffix(name, exeSuffix) {
			name += exeSuffix
		}
	}
	return path.Join(filepath.ToSlash(targetFolder), name)
}

func isRemotePath(p string) bool {
	return strings.HasPrefix(p, httpPrefix) || strings.HasPrefix(p, httpsPrefix)
}

func downloadBinary(
	binaryName string,
	binary *provider2.ProviderBinary,
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
		return "", fmt.Errorf("create folder %w", err)
	}

	return downloadRemoteBinary(binaryName, binary, targetFolder, log)
}

func handleLocalBinary(binary *provider2.ProviderBinary, targetFolder string) (string, error) {
	if filepath.IsAbs(binary.Path) {
		return binary.Path, nil
	}

	if err := os.MkdirAll(targetFolder, dirPerms); err != nil {
		return "", fmt.Errorf("create folder %w", err)
	}

	targetPath := localTargetPath(binary, targetFolder)
	if err := copyLocal(binary, targetPath); err != nil {
		_ = os.Remove(targetPath)
		return "", err
	}

	return targetPath, nil
}

func handleNonHTTPBinary(binary *provider2.ProviderBinary, targetFolder string) (string, error) {
	targetPath := localTargetPath(binary, targetFolder)
	if _, err := os.Stat(targetPath); err == nil {
		return targetPath, nil
	}
	return "", fmt.Errorf("cannot download %s as scheme is missing", binary.Path)
}

func downloadRemoteBinary(
	binaryName string,
	binary *provider2.ProviderBinary,
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
	binary *provider2.ProviderBinary,
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
	targetPath := path.Join(filepath.ToSlash(targetFolder), name)
	_, err := os.Stat(targetPath)
	if err == nil {
		return targetPath, nil
	}

	return downloadAndSaveFile(binaryName, binary, targetPath, log)
}

func downloadAndSaveFile(
	binaryName string,
	binary *provider2.ProviderBinary,
	targetPath string,
	log log.Logger,
) (string, error) {
	log.Infof("downloading binary %s from %s", binaryName, binary.Path)

	body, err := download.File(binary.Path, log)
	if err != nil {
		return "", fmt.Errorf("download binary %w", err)
	}
	defer func() { _ = body.Close() }()

	// #nosec G304 - targetPath is constructed from validated binary configuration
	file, err := os.Create(targetPath)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	if _, err := io.Copy(file, body); err != nil {
		return "", fmt.Errorf("download file %w", err)
	}

	log.Debugf("downloaded binary %s", binaryName)
	return targetPath, nil
}

func downloadArchive(
	binaryName string,
	binary *provider2.ProviderBinary,
	targetFolder string,
	log log.Logger,
) (string, error) {
	targetPath := path.Join(filepath.ToSlash(targetFolder), binary.ArchivePath)
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
	binary       *provider2.ProviderBinary
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

	isGzipOrTar := strings.HasSuffix(params.binary.Path, gzSuffix) ||
		strings.HasSuffix(params.binary.Path, tarSuffix) ||
		strings.HasSuffix(params.binary.Path, tgzSuffix)

	if isGzipOrTar {
		if err := extract.Extract(body, params.targetFolder); err != nil {
			return "", err
		}
		params.log.Debugf("extracted and downloaded gz or tar archive %s", params.binaryName)
		return params.targetPath, nil
	}

	if strings.HasSuffix(params.binary.Path, zipSuffix) {
		targetPath, err := extractZipArchive(body, params.targetFolder, params.targetPath)
		if err != nil {
			return "", err
		}
		params.log.Debugf("extracted and downloaded zip archive %s", params.binaryName)
		return targetPath, nil
	}

	return "", fmt.Errorf("unrecognized archive format %s", params.binary.Path)
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

func localTargetPath(binary *provider2.ProviderBinary, targetFolder string) string {
	name := binary.Name
	if name == "" {
		name = path.Base(binary.Path)
	}
	return filepath.Join(targetFolder, name)
}

func copyLocal(binary *provider2.ProviderBinary, targetPath string) error {
	targetPathStat, err := os.Stat(targetPath)
	if err == nil {
		binaryStat, err := os.Stat(binary.Path)
		if err != nil {
			return err
		}
		if targetPathStat.Size() == binaryStat.Size() {
			return nil
		}
	}

	return copy.File(binary.Path, targetPath, filePerms)
}
