package agent

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/loft-sh/log"
	devpodhttp "github.com/skevetter/devpod/pkg/http"
	"github.com/skevetter/devpod/pkg/inject"
	"github.com/skevetter/devpod/pkg/shell"
	"github.com/skevetter/devpod/pkg/version"
)

var waitForInstanceConnectionTimeout = time.Minute * 5

// progressReader wraps an io.Reader to provide download progress logging
type progressReader struct {
	reader  io.Reader
	total   int64
	read    int64
	log     log.Logger
	start   time.Time
	lastLog time.Time
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	pr.read += int64(n)

	// Log progress every 10 seconds or at completion
	now := time.Now()
	if now.Sub(pr.lastLog) >= 10*time.Second || err == io.EOF {
		pr.lastLog = now
		if pr.total > 0 {
			percent := float64(pr.read) / float64(pr.total) * 100
			elapsed := now.Sub(pr.start)
			speed := float64(pr.read) / (1024 * 1024) / elapsed.Seconds() // MB/s
			pr.log.Infof("download progress %.1f%% (%d/%d bytes, %.2f MB/s)",
				percent, pr.read, pr.total, speed)
		}
	}

	return n, err
}

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

		log.Debugf("Execute command locally")
		return shell.RunEmulatedShell(ctx, command, stdin, stdout, stderr, nil)
	}

	defer log.Debugf("Done InjectAgentAndExecute")
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
				log.Infof("Waiting for devpod agent to come up...")
				lastMessage = time.Now()
			}

			log.Debugf("Inject Error: %s%v", buf.String(), err)
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
	}

	// try to look up runner binaries
	if binaryPath == "" {
		binaryPath = getRunnerBinary(targetArch)
		if binaryPath != "" {
			log.Infof("Using cached agent binary: %s", binaryPath)
		}
	}

	// download devpod locally
	if binaryPath == "" {
		log.Infof("Downloading agent binary for %s architecture", targetArch)
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

func downloadAgentLocally(tryDownloadURL, targetArch string, log log.Logger) (string, error) {
	agentPath := filepath.Join(os.TempDir(), "devpod-cache", "devpod-linux-"+targetArch)
	err := os.MkdirAll(filepath.Dir(agentPath), 0755)
	if err != nil {
		return "", fmt.Errorf("create agent path %w", err)
	}

	stat, statErr := os.Stat(agentPath)
	if version.GetVersion() == version.DevVersion && statErr == nil {
		return agentPath, nil
	}

	fullDownloadURL := tryDownloadURL + "/devpod-linux-" + targetArch
	log.Debugf("Attempting to download DevPod agent from: %s", fullDownloadURL)

	resp, err := devpodhttp.GetHTTPClient().Get(fullDownloadURL)
	if err != nil {
		return "", fmt.Errorf("download devpod %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if statErr == nil && stat.Size() == resp.ContentLength {
		return agentPath, nil
	}

	log.Infof("Download DevPod Agent...")
	file, err := os.Create(agentPath)
	if err != nil {
		return "", fmt.Errorf("create agent binary %w", err)
	}
	defer func() { _ = file.Close() }()

	// Track download progress
	startTime := time.Now()
	log.Infof("Starting agent download from %s (size: %d bytes)", fullDownloadURL, resp.ContentLength)

	// Create a progress reader
	progressReader := &progressReader{
		reader: resp.Body,
		total:  resp.ContentLength,
		log:    log,
		start:  startTime,
	}

	_, err = io.Copy(file, progressReader)
	if err != nil {
		_ = os.Remove(agentPath)
		return "", fmt.Errorf("failed to download devpod from URL %s %w", fullDownloadURL, err)
	}

	duration := time.Since(startTime)
	log.Infof("Agent download completed in %v (%.2f MB/s)", duration, float64(resp.ContentLength)/(1024*1024)/duration.Seconds())

	return agentPath, nil
}

func getRunnerBinary(targetArch string) string {
	binaryPath := filepath.Join(os.TempDir(), "devpod-cache", "devpod-linux-"+targetArch)
	_, err := os.Stat(binaryPath)
	if err != nil {
		return ""
	}
	return binaryPath
}
