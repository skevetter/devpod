package agent

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	devpodhttp "github.com/skevetter/devpod/pkg/http"
	"github.com/skevetter/devpod/pkg/inject"
	"github.com/skevetter/devpod/pkg/shell"
	"github.com/skevetter/devpod/pkg/version"
	"github.com/skevetter/log"
)

var waitForInstanceConnectionTimeout = time.Minute * 5

func InjectAgent(
	ctx context.Context,
	exec inject.ExecFunc,
	local bool,
	remoteAgentPath,
	downloadURL string,
	preferDownload bool,
	log log.Logger,
	timeout time.Duration,
) error {
	return InjectAgentAndExecute(
		ctx,
		exec,
		local,
		remoteAgentPath,
		downloadURL,
		preferDownload,
		"",
		nil,
		nil,
		nil,
		log,
		timeout,
	)
}

func InjectAgentAndExecute(
	ctx context.Context,
	exec inject.ExecFunc,
	local bool,
	remoteAgentPath,
	downloadURL string,
	preferDownload bool,
	command string,
	stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer,
	log log.Logger,
	timeout time.Duration,
) error {
	// should execute locally?
	if local {
		if command == "" {
			return nil
		}

		log.Debug("execute command locally")
		return shell.RunEmulatedShell(ctx, command, stdin, stdout, stderr, nil)
	}

	defer log.Debug("done InjectAgentAndExecute")
	if remoteAgentPath == "" {
		remoteAgentPath = RemoteDevPodHelperLocation
	}
	if downloadURL == "" {
		downloadURL = DefaultAgentDownloadURL()
	}

	versionCheck := fmt.Sprintf(`[ "$(%s version 2>/dev/null || echo 'false')" != "%s" ]`, remoteAgentPath, version.GetVersion())
	if version.GetVersion() == version.DevVersion {
		preferDownload = false
	}

	// install devpod into the target
	// do a simple hello world to check if we can get something
	now := time.Now()
	lastMessage := time.Now()
	for {
		buf := &bytes.Buffer{}
		if stderr != nil {
			stderr = io.MultiWriter(stderr, buf)
		} else {
			stderr = buf
		}

		scriptParams := &inject.Params{
			Command:             command,
			AgentRemotePath:     remoteAgentPath,
			DownloadURLs:        inject.NewDownloadURLs(downloadURL),
			ExistsCheck:         versionCheck,
			PreferAgentDownload: preferDownload,
			ShouldChmodPath:     true,
		}

		wasExecuted, err := inject.InjectAndExecute(
			ctx,
			exec,
			func(arm bool) (io.ReadCloser, error) {
				return injectBinary(arm, downloadURL, log)
			},
			scriptParams,
			stdin,
			stdout,
			stderr,
			timeout,
			log,
		)
		if err != nil {
			if time.Since(now) > waitForInstanceConnectionTimeout {
				return fmt.Errorf("timeout waiting for instance connection %w", err)
			} else if wasExecuted {
				return fmt.Errorf("agent error: %s %w", buf.String(), err)
			}

			if time.Since(lastMessage) > time.Second*5 {
				log.Info("waiting for devpod agent to come up")
				lastMessage = time.Now()
			}

			log.WithFields(logrus.Fields{
				"error":  err,
				"output": buf.String(),
			}).Debug("inject error")
			time.Sleep(time.Second * 3)
			continue
		}

		break
	}

	return nil
}

func injectBinary(arm bool, tryDownloadURL string, log log.Logger) (io.ReadCloser, error) {
	// this means we need to
	targetArch := "amd64"
	if arm {
		targetArch = "arm64"
	}

	// make sure a linux arm64 binary exists locally
	var err error
	var binaryPath string
	if runtime.GOOS == "linux" && runtime.GOARCH == targetArch {
		binaryPath, err = os.Executable()
		if err != nil {
			return nil, fmt.Errorf("get executable %w", err)
		}

		// check if we still exist
		_, err = os.Stat(binaryPath)
		if err != nil {
			binaryPath = ""
		}

		log.WithFields(logrus.Fields{
			"path": binaryPath,
			"arch": targetArch,
		}).Debug("using current binary for agent injection")

		// Validate binary compatibility
		if err := validateBinaryCompatibility(binaryPath, log); err != nil {
			log.WithFields(logrus.Fields{
				"path":  binaryPath,
				"error": err,
			}).Warn("current binary compatibility check failed")
		}
	}

	// try to look up runner binaries
	if binaryPath == "" {
		binaryPath = getRunnerBinary(targetArch)
	}

	if binaryPath != "" {
		log.WithFields(logrus.Fields{
			"path": binaryPath,
			"arch": targetArch,
			"url":  tryDownloadURL,
		}).Debug("found runner binary for agent injection")

		// Validate binary compatibility in CI
		if err := validateBinaryCompatibility(binaryPath, log); err != nil {
			log.WithFields(logrus.Fields{
				"path":  binaryPath,
				"error": err,
			}).Warn("binary compatibility check failed")
		}
	}

	// download devpod locally
	if binaryPath == "" {
		binaryPath, err = downloadAgentLocally(tryDownloadURL, targetArch, log)
		if err != nil {
			return nil, fmt.Errorf("download agent locally %w", err)
		}
	}

	// read file
	file, err := os.Open(binaryPath)
	if err != nil {
		return nil, fmt.Errorf("open agent binary %w", err)
	}

	return file, nil
}

func validateBinaryCompatibility(binaryPath string, log log.Logger) error {
	// Check if binary is static (no dynamic dependencies)
	cmd := exec.Command("ldd", binaryPath)
	output, err := cmd.Output()

	if err != nil {
		// ldd failed, might be static or not available
		log.WithFields(logrus.Fields{
			"path":  binaryPath,
			"error": err,
		}).Debug("ldd command failed, assuming binary is compatible")
		return nil
	}

	outputStr := string(output)
	if strings.Contains(outputStr, "not a dynamic executable") || strings.Contains(outputStr, "statically linked") {
		log.WithFields(logrus.Fields{
			"path": binaryPath,
		}).Debug("binary is statically linked")
		return nil
	}

	// Check for problematic GLIBC versions
	if strings.Contains(outputStr, "GLIBC_2.3") && (strings.Contains(outputStr, "2.38") || strings.Contains(outputStr, "2.39")) {
		log.WithFields(logrus.Fields{
			"path":         binaryPath,
			"dependencies": strings.TrimSpace(outputStr),
		}).Error("binary requires newer GLIBC version, will cause compatibility issues")
		return fmt.Errorf("binary requires GLIBC 2.38+ which may not be available in target containers")
	}

	// Binary has dynamic dependencies, log them for debugging
	log.WithFields(logrus.Fields{
		"path":         binaryPath,
		"dependencies": strings.TrimSpace(outputStr),
	}).Warn("binary has dynamic dependencies, may cause compatibility issues")

	return nil
}

func downloadAgentLocally(tryDownloadURL, targetArch string, log log.Logger) (string, error) {
	agentPath := filepath.Join(os.TempDir(), "devpod-cache", "devpod-linux-"+targetArch)
	log.WithFields(logrus.Fields{
		"path": agentPath,
	}).Debug("checking for devpod agent binary")

	// create path
	err := os.MkdirAll(filepath.Dir(agentPath), 0755)
	if err != nil {
		return "", fmt.Errorf("create agent path %w", err)
	}
	log.WithFields(logrus.Fields{
		"path": filepath.Dir(agentPath),
	}).Debug("created agent path")

	stat, statErr := os.Stat(agentPath)
	if version.GetVersion() == version.DevVersion && statErr == nil {
		return agentPath, nil
	}

	fullDownloadURL := tryDownloadURL + "/devpod-linux-" + targetArch
	log.WithFields(logrus.Fields{
		"url": fullDownloadURL,
	}).Debug("attempting to download devpod agent")

	resp, err := devpodhttp.GetHTTPClient().Get(fullDownloadURL)
	if err != nil {
		return "", fmt.Errorf("download devpod %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if statErr == nil && stat.Size() == resp.ContentLength {
		return agentPath, nil
	}

	log.Info("download devpod agent binary")
	file, err := os.Create(agentPath)
	if err != nil {
		return "", fmt.Errorf("create agent binary %w", err)
	}
	log.WithFields(logrus.Fields{
		"path": agentPath,
	}).Debug("created agent binary")
	defer func() { _ = file.Close() }()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		_ = os.Remove(agentPath)
		log.WithFields(logrus.Fields{
			"error": err,
			"url":   fullDownloadURL,
		}).Error("failed to download devpod agent")
		return "", fmt.Errorf("failed to download devpod from URL %s %w", fullDownloadURL, err)
	}

	return agentPath, nil
}

func getRunnerBinary(targetArch string) string {
	binaryPath := filepath.Join(os.TempDir(), "devpod-cache", "devpod-linux-"+targetArch)
	_, err := os.Stat(binaryPath)
	if err != nil {
		return ""
	}
	log.WithFields(logrus.Fields{
		"path": binaryPath,
	}).Debug("found runner binary in devpod-cache")
	return binaryPath
}
