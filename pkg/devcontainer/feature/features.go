package feature

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	pkgconfig "github.com/skevetter/devpod/pkg/config"
	"github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/skevetter/devpod/pkg/extract"
	devpodhttp "github.com/skevetter/devpod/pkg/http"
	"github.com/skevetter/log"
	"github.com/skevetter/log/hash"
)

const DEVCONTAINER_MANIFEST_MEDIATYPE = "application/vnd.devcontainers"

var directTarballRegEx = regexp.MustCompile("devcontainer-feature-([a-zA-Z0-9_-]+).tgz")

func getFeatureInstallWrapperScript(
	idWithoutVersion string,
	feature *config.FeatureConfig,
	options []string,
) string {
	id := escapeQuotesForShell(idWithoutVersion)
	name := escapeQuotesForShell(feature.Name)
	description := escapeQuotesForShell(feature.Description)
	version := escapeQuotesForShell(feature.Version)
	documentation := escapeQuotesForShell(feature.DocumentationURL)
	optionsIndented := escapeQuotesForShell("    " + strings.Join(options, "\n    "))

	warningHeader := ""
	if feature.Deprecated {
		warningHeader += `(!) WARNING: Using the deprecated Feature ` +
			`"${escapeQuotesForShell(feature.id)}". This Feature will no longer receive any further updates/support.\n`
	}

	echoWarning := ""
	if warningHeader != "" {
		echoWarning = `echo '` + warningHeader + `'`
	}

	errorMessage := `ERROR: Feature "` + name + `" (` + id + `) failed to install!`
	troubleshootingMessage := ""
	if documentation != "" {
		troubleshootingMessage = ` Look at the documentation at ${documentation} for help troubleshooting this error.`
	}

	return `#!/bin/sh
set -e

on_exit () {
	[ $? -eq 0 ] && exit
	echo '` + errorMessage + troubleshootingMessage + `'
}

trap on_exit EXIT

set -a
. ../devcontainer-features.builtin.env
. ./devcontainer-features.env
set +a

echo ===========================================================================
` + echoWarning + `
echo 'Feature       : ` + name + `'
echo 'Description   : ` + description + `'
echo 'Id            : ` + id + `'
echo 'Version       : ` + version + `'
echo 'Documentation : ` + documentation + `'
echo 'Options       :'
echo '` + optionsIndented + `'
echo 'Environment   :'
printenv
echo ===========================================================================

chmod +x ./install.sh
./install.sh
`
}

func escapeQuotesForShell(str string) string {
	// The `input` is expected to be a string which will be printed inside single quotes
	// by the caller. This means we need to escape any nested single quotes within the string.
	// We can do this by ending the first string with a single quote ('), printing an escaped
	// single quote (\'), and then opening a new string (').
	return strings.ReplaceAll(str, "'", `'\''`)
}

func ProcessFeatureID(
	id string,
	devContainerConfig *config.DevContainerConfig,
	log log.Logger,
	forceBuild bool,
) (string, error) {
	if strings.HasPrefix(id, "https://") || strings.HasPrefix(id, "http://") {
		log.Debugf("process feature: type=%s, id=%s", "url", id)
		return processDirectTarFeature(
			id,
			config.GetDevPodCustomizations(devContainerConfig).FeatureDownloadHTTPHeaders,
			log,
			forceBuild,
		)
	} else if strings.HasPrefix(id, "./") || strings.HasPrefix(id, "../") {
		log.Debugf("process feature: type=%s, id=%s", "local", id)
		return filepath.Abs(
			path.Join(filepath.ToSlash(filepath.Dir(devContainerConfig.Origin)), id),
		)
	}

	// get oci feature
	log.Debugf("process feature: type=%s, id=%s", "oci", id)
	return processOCIFeature(id, log)
}

func processOCIFeature(id string, log log.Logger) (string, error) {
	log.Debugf("processing OCI feature: featureId=%s", id)

	// feature already exists?
	featureFolder := getFeaturesTempFolder(id)
	featureExtractedFolder := filepath.Join(featureFolder, "extracted")
	_, err := os.Stat(featureExtractedFolder)
	if err == nil {
		// make sure feature.json is there as well
		_, err = os.Stat(
			filepath.Join(featureExtractedFolder, config.DEVCONTAINER_FEATURE_FILE_NAME),
		)
		if err == nil {
			log.Debugf("feature already cached: folder=%s", featureExtractedFolder)
			return featureExtractedFolder, nil
		} else {
			log.Debugf("feature folder exists but seems empty: folder=%s", featureExtractedFolder)
			_ = os.RemoveAll(featureFolder)
		}
	}

	ref, err := name.ParseReference(id)
	if err != nil {
		log.Errorf("failed to parse OCI reference: error=%v, featureId=%s", err, id)
		return "", err
	}

	log.Debugf("fetching OCI image: reference=%s", ref.String())
	img, err := remote.Image(ref, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		log.Errorf("failed to fetch OCI image: error=%v, reference=%s", err, ref.String())
		return "", err
	}

	destFile := filepath.Join(featureFolder, "feature.tgz")
	err = downloadLayer(img, id, destFile, log)
	if err != nil {
		log.Errorf("failed to download feature layer: error=%v, featureId=%s", err, id)
		return "", err
	}

	file, err := os.Open(destFile)
	if err != nil {
		log.Errorf("failed to open downloaded feature file: error=%v, file=%s", err, destFile)
		return "", err
	}
	defer func() { _ = file.Close() }()

	log.Debugf("extract feature: destination=%s", featureExtractedFolder)
	err = extract.Extract(file, featureExtractedFolder)
	if err != nil {
		log.Errorf(
			"failed to extract feature: error=%v, destination=%s",
			err,
			featureExtractedFolder,
		)
		_ = os.RemoveAll(featureExtractedFolder)
		return "", err
	}

	log.Infof(
		"OCI feature processed successfully: featureId=%s, path=%s",
		id,
		featureExtractedFolder,
	)
	return featureExtractedFolder, nil
}

func downloadLayer(img v1.Image, id, destFile string, log log.Logger) error {
	manifest, err := img.Manifest()
	if err != nil {
		return err
	} else if manifest.Config.MediaType != DEVCONTAINER_MANIFEST_MEDIATYPE {
		return fmt.Errorf(
			"incorrect manifest type %s, expected %s",
			manifest.Config.MediaType,
			DEVCONTAINER_MANIFEST_MEDIATYPE,
		)
	} else if len(manifest.Layers) == 0 {
		return fmt.Errorf("unexpected amount of layers, expected at least 1")
	}

	// download layer
	log.Debugf(
		"download feature layer: featureId=%s, digest=%s, destFile=%s",
		id,
		manifest.Layers[0].Digest.String(),
		destFile,
	)
	layer, err := img.LayerByDigest(manifest.Layers[0].Digest)
	if err != nil {
		return fmt.Errorf("retrieve layer: %w", err)
	}

	data, err := layer.Uncompressed()
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer func() { _ = data.Close() }()

	// #nosec G301 -- TODO Consider using a more secure permission setting and ownership if needed.
	err = os.MkdirAll(filepath.Dir(destFile), 0o755)
	if err != nil {
		return fmt.Errorf("create target folder: %w", err)
	}

	file, err := os.Create(destFile)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer func() { _ = file.Close() }()

	_, err = io.Copy(file, data)
	if err != nil {
		return fmt.Errorf("download layer: %w", err)
	}

	return nil
}

func processDirectTarFeature(
	id string,
	httpHeaders map[string]string,
	log log.Logger,
	forceDownload bool,
) (string, error) {
	log.Debugf("processing direct tar feature: featureId=%s, forceDownload=%v", id, forceDownload)

	downloadBase := id[strings.LastIndex(id, "/"):]
	if !directTarballRegEx.MatchString(downloadBase) {
		log.Errorf("invalid tarball filename format: filename=%s", downloadBase)
		return "", fmt.Errorf(
			"expected tarball name to follow 'devcontainer-feature-<feature-id>.tgz' format.  Received '%s' ",
			downloadBase,
		)
	}

	// feature already exists?
	featureFolder := getFeaturesTempFolder(id)
	featureExtractedFolder := filepath.Join(featureFolder, "extracted")
	_, err := os.Stat(featureExtractedFolder)
	if err == nil && !forceDownload {
		log.Debugf("direct tar feature already cached: folder=%s", featureExtractedFolder)
		return featureExtractedFolder, nil
	}

	// download feature tarball
	downloadFile := filepath.Join(featureFolder, "feature.tgz")
	err = downloadFeatureFromURL(id, downloadFile, httpHeaders, log)
	if err != nil {
		log.Errorf("failed to download feature tarball: error=%v, url=%s", err, id)
		return "", err
	}

	// extract file
	file, err := os.Open(downloadFile)
	if err != nil {
		log.Errorf("failed to open downloaded tarball: error=%v, file=%s", err, downloadFile)
		return "", err
	}
	defer func() { _ = file.Close() }()

	// extract tar.gz
	err = extract.Extract(file, featureExtractedFolder)
	if err != nil {
		log.Errorf(
			"failed to extract tarball: error=%v, destination=%s",
			err,
			featureExtractedFolder,
		)
		_ = os.RemoveAll(featureExtractedFolder)
		return "", fmt.Errorf("extract folder: %w", err)
	}

	log.Infof(
		"Direct tar feature processed successfully: featureId=%s, path=%s",
		id,
		featureExtractedFolder,
	)
	return featureExtractedFolder, nil
}

func downloadFeatureFromURL(
	url string,
	destFile string,
	httpHeaders map[string]string,
	log log.Logger,
) error {
	log.Debugf("starting feature download: url=%s, destFile=%s", url, destFile)

	// #nosec G301 -- TODO Consider using a more secure permission setting and ownership if needed.
	err := os.MkdirAll(filepath.Dir(destFile), 0o755)
	if err != nil {
		log.Errorf("failed to create feature folder: error=%v, dir=%s", err, filepath.Dir(destFile))
		return fmt.Errorf("create feature folder: %w", err)
	}

	attempt := 0
	for range 3 {
		if attempt > 0 {
			delay := time.Duration(1<<uint(attempt-1)) * time.Second
			log.Debugf("retrying download: delay=%v, attempt=%v", delay, attempt)
			time.Sleep(delay)
		}

		log.Debugf("download feature: url=%s", url)
		if err := tryDownload(url, destFile, httpHeaders); err != nil {
			if attempt == 2 {
				log.Errorf("all download attempts failed: error=%v, url=%s", err, url)
				return err
			}
			log.Debugf("download attempt failed: error=%v, attempt=%v", err, attempt)
			attempt++
			continue
		}
		log.Infof("Feature download completed successfully: url=%s, destFile=%s", url, destFile)
		return nil
	}

	return fmt.Errorf("download failed")
}

func tryDownload(url, destFile string, httpHeaders map[string]string) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("make request: %w", err)
	}
	for key, value := range httpHeaders {
		req.Header.Set(key, value)
	}

	resp, err := devpodhttp.GetHTTPClient().Do(req)
	if err != nil {
		return fmt.Errorf("make request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("GET request failed, status code is %d", resp.StatusCode)
	}

	file, err := os.Create(destFile)
	if err != nil {
		return fmt.Errorf("create download file: %w", err)
	}
	defer func() { _ = file.Close() }()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return fmt.Errorf("download feature: %w", err)
	}

	return nil
}

func getFeaturesTempFolder(id string) string {
	hashedID := hash.String(id)[:10]
	return filepath.Join(os.TempDir(), pkgconfig.BinaryName, "features", hashedID)
}
