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
	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/skevetter/devpod/pkg/extract"
	devpodhttp "github.com/skevetter/devpod/pkg/http"
	"github.com/skevetter/log"
	"github.com/skevetter/log/hash"
)

const DEVCONTAINER_MANIFEST_MEDIATYPE = "application/vnd.devcontainers"

var directTarballRegEx = regexp.MustCompile("devcontainer-feature-([a-zA-Z0-9_-]+).tgz")

func getFeatureInstallWrapperScript(idWithoutVersion string, feature *config.FeatureConfig, options []string) string {
	id := escapeQuotesForShell(idWithoutVersion)
	name := escapeQuotesForShell(feature.Name)
	description := escapeQuotesForShell(feature.Description)
	version := escapeQuotesForShell(feature.Version)
	documentation := escapeQuotesForShell(feature.DocumentationURL)
	optionsIndented := escapeQuotesForShell("    " + strings.Join(options, "\n    "))

	warningHeader := ""
	if feature.Deprecated {
		warningHeader += `(!) WARNING: Using the deprecated Feature "${escapeQuotesForShell(feature.id)}". This Feature will no longer receive any further updates/support.\n`
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

func ProcessFeatureID(id string, devContainerConfig *config.DevContainerConfig, log log.Logger, forceBuild bool) (string, error) {
	if strings.HasPrefix(id, "https://") || strings.HasPrefix(id, "http://") {
		log.WithFields(logrus.Fields{"type": "url", "id": id}).Debug("process feature")
		return processDirectTarFeature(id, config.GetDevPodCustomizations(devContainerConfig).FeatureDownloadHTTPHeaders, log, forceBuild)
	} else if strings.HasPrefix(id, "./") || strings.HasPrefix(id, "../") {
		log.WithFields(logrus.Fields{"type": "local", "id": id}).Debug("process feature")
		return filepath.Abs(path.Join(filepath.ToSlash(filepath.Dir(devContainerConfig.Origin)), id))
	}

	// get oci feature
	log.WithFields(logrus.Fields{"type": "oci", "id": id}).Debug("process feature")
	return processOCIFeature(id, log)
}

func processOCIFeature(id string, log log.Logger) (string, error) {
	log.WithFields(logrus.Fields{"featureId": id}).Debug("processing OCI feature")

	// feature already exists?
	featureFolder := getFeaturesTempFolder(id)
	featureExtractedFolder := filepath.Join(featureFolder, "extracted")
	_, err := os.Stat(featureExtractedFolder)
	if err == nil {
		// make sure feature.json is there as well
		_, err = os.Stat(filepath.Join(featureExtractedFolder, config.DEVCONTAINER_FEATURE_FILE_NAME))
		if err == nil {
			log.WithFields(logrus.Fields{"folder": featureExtractedFolder}).Debug("feature already cached")
			return featureExtractedFolder, nil
		} else {
			log.WithFields(logrus.Fields{"folder": featureExtractedFolder}).Debug("feature folder exists but seems empty")
			_ = os.RemoveAll(featureFolder)
		}
	}

	ref, err := name.ParseReference(id)
	if err != nil {
		log.WithFields(logrus.Fields{"error": err, "featureId": id}).Error("failed to parse OCI reference")
		return "", err
	}

	log.WithFields(logrus.Fields{"reference": ref.String()}).Debug("fetching OCI image")
	img, err := remote.Image(ref, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		log.WithFields(logrus.Fields{"error": err, "reference": ref.String()}).Error("failed to fetch OCI image")
		return "", err
	}

	destFile := filepath.Join(featureFolder, "feature.tgz")
	err = downloadLayer(img, id, destFile, log)
	if err != nil {
		log.WithFields(logrus.Fields{"error": err, "featureId": id}).Error("failed to download feature layer")
		return "", err
	}

	file, err := os.Open(destFile)
	if err != nil {
		log.WithFields(logrus.Fields{"error": err, "file": destFile}).Error("failed to open downloaded feature file")
		return "", err
	}
	defer func() { _ = file.Close() }()

	log.WithFields(logrus.Fields{"destination": featureExtractedFolder}).Debug("extract feature")
	err = extract.Extract(file, featureExtractedFolder)
	if err != nil {
		log.WithFields(logrus.Fields{"error": err, "destination": featureExtractedFolder}).Error("failed to extract feature")
		_ = os.RemoveAll(featureExtractedFolder)
		return "", err
	}

	log.WithFields(logrus.Fields{"featureId": id, "path": featureExtractedFolder}).Info("OCI feature processed successfully")
	return featureExtractedFolder, nil
}

func downloadLayer(img v1.Image, id, destFile string, log log.Logger) error {
	manifest, err := img.Manifest()
	if err != nil {
		return err
	} else if manifest.Config.MediaType != DEVCONTAINER_MANIFEST_MEDIATYPE {
		return fmt.Errorf("incorrect manifest type %s, expected %s", manifest.Config.MediaType, DEVCONTAINER_MANIFEST_MEDIATYPE)
	} else if len(manifest.Layers) == 0 {
		return fmt.Errorf("unexpected amount of layers, expected at least 1")
	}

	// download layer
	log.WithFields(logrus.Fields{
		"featureId": id,
		"digest":    manifest.Layers[0].Digest.String(),
		"destFile":  destFile,
	}).Debug("download feature layer")
	layer, err := img.LayerByDigest(manifest.Layers[0].Digest)
	if err != nil {
		return fmt.Errorf("retrieve layer: %w", err)
	}

	data, err := layer.Uncompressed()
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer func() { _ = data.Close() }()

	err = os.MkdirAll(filepath.Dir(destFile), 0755)
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

func processDirectTarFeature(id string, httpHeaders map[string]string, log log.Logger, forceDownload bool) (string, error) {
	log.WithFields(logrus.Fields{"featureId": id, "forceDownload": forceDownload}).Debug("processing direct tar feature")

	downloadBase := id[strings.LastIndex(id, "/"):]
	if !directTarballRegEx.MatchString(downloadBase) {
		log.WithFields(logrus.Fields{"filename": downloadBase}).Error("invalid tarball filename format")
		return "", fmt.Errorf("expected tarball name to follow 'devcontainer-feature-<feature-id>.tgz' format.  Received '%s' ", downloadBase)
	}

	// feature already exists?
	featureFolder := getFeaturesTempFolder(id)
	featureExtractedFolder := filepath.Join(featureFolder, "extracted")
	_, err := os.Stat(featureExtractedFolder)
	if err == nil && !forceDownload {
		log.WithFields(logrus.Fields{"folder": featureExtractedFolder}).Debug("direct tar feature already cached")
		return featureExtractedFolder, nil
	}

	// download feature tarball
	downloadFile := filepath.Join(featureFolder, "feature.tgz")
	err = downloadFeatureFromURL(id, downloadFile, httpHeaders, log)
	if err != nil {
		log.WithFields(logrus.Fields{"error": err, "url": id}).Error("failed to download feature tarball")
		return "", err
	}

	// extract file
	file, err := os.Open(downloadFile)
	if err != nil {
		log.WithFields(logrus.Fields{"error": err, "file": downloadFile}).Error("failed to open downloaded tarball")
		return "", err
	}
	defer func() { _ = file.Close() }()

	// extract tar.gz
	err = extract.Extract(file, featureExtractedFolder)
	if err != nil {
		log.WithFields(logrus.Fields{"error": err, "destination": featureExtractedFolder}).Error("failed to extract tarball")
		_ = os.RemoveAll(featureExtractedFolder)
		return "", fmt.Errorf("extract folder: %w", err)
	}

	log.WithFields(logrus.Fields{"featureId": id, "path": featureExtractedFolder}).Info("Direct tar feature processed successfully")
	return featureExtractedFolder, nil
}

func downloadFeatureFromURL(url string, destFile string, httpHeaders map[string]string, log log.Logger) error {
	log.WithFields(logrus.Fields{"url": url, "destFile": destFile}).Debug("starting feature download")

	err := os.MkdirAll(filepath.Dir(destFile), 0755)
	if err != nil {
		log.WithFields(logrus.Fields{"error": err, "dir": filepath.Dir(destFile)}).Error("failed to create feature folder")
		return fmt.Errorf("create feature folder: %w", err)
	}

	attempt := 0
	for range 3 {
		if attempt > 0 {
			delay := time.Duration(1<<uint(attempt-1)) * time.Second
			log.WithFields(logrus.Fields{"delay": delay, "attempt": attempt}).Debug("retrying download")
			time.Sleep(delay)
		}

		log.WithFields(logrus.Fields{"url": url}).Debug("download feature")
		if err := tryDownload(url, destFile, httpHeaders); err != nil {
			if attempt == 2 {
				log.WithFields(logrus.Fields{"error": err, "url": url}).Error("all download attempts failed")
				return err
			}
			log.WithFields(logrus.Fields{"error": err, "attempt": attempt}).Debug("download attempt failed")
			attempt++
			continue
		}
		log.WithFields(logrus.Fields{"url": url, "destFile": destFile}).Info("Feature download completed successfully")
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
	return filepath.Join(os.TempDir(), "devpod", "features", hashedID)
}
