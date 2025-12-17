package jetbrains

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"

	"github.com/loft-sh/log"
	"github.com/pkg/browser"
	"github.com/skevetter/devpod/pkg/command"
	"github.com/skevetter/devpod/pkg/config"
	copy2 "github.com/skevetter/devpod/pkg/copy"
	"github.com/skevetter/devpod/pkg/extract"
	devpodhttp "github.com/skevetter/devpod/pkg/http"
	"github.com/skevetter/devpod/pkg/ide"
	"github.com/skevetter/devpod/pkg/util"
)

const (
	VersionOption       = "VERSION"
	DownloadAmd64Option = "DOWNLOAD_AMD64"
	DownloadArm64Option = "DOWNLOAD_ARM64"
)

func getLatestDownloadURL(code string, platform string) string {
	return fmt.Sprintf("https://download.jetbrains.com/product?code=%s&platform=%s", code, platform)
}

func getDownloadURLs(options ide.Options, values map[string]config.OptionValue, productCode string, templateAmd64 string, templateArm64 string) (string, string) {
	version := options.GetValue(values, VersionOption)
	var amd64Download, arm64Download string
	if version == "latest" {
		amd64Download = getLatestDownloadURL(productCode, "linux")
		arm64Download = getLatestDownloadURL(productCode, "linuxARM64")
	} else {
		amd64Download = options.GetValue(values, DownloadAmd64Option)
		if amd64Download == "" {
			amd64Download = fmt.Sprintf(templateAmd64, version)
		}
		arm64Download = options.GetValue(values, DownloadArm64Option)
		if arm64Download == "" {
			arm64Download = fmt.Sprintf(templateArm64, version)
		}
	}

	return amd64Download, arm64Download
}

type GenericOptions struct {
	ID          string
	DisplayName string

	DownloadAmd64 string
	DownloadArm64 string
}

func newGenericServer(userName string, options *GenericOptions, log log.Logger) *GenericJetBrainsServer {
	return &GenericJetBrainsServer{
		userName: userName,
		options:  options,
		log:      log,
	}
}

type GenericJetBrainsServer struct {
	userName string
	options  *GenericOptions
	log      log.Logger
}

func (o *GenericJetBrainsServer) OpenGateway(workspaceFolder, workspaceID string) error {
	o.log.Infof("starting %s through JetBrains Gateway", o.options.DisplayName)

	// Build JetBrains Gateway URL
	// Reference: https://www.jetbrains.com/help/idea/remote-development-a.html#use_idea
	// Format: jetbrains-gateway://connect#idePath=<path>&projectPath=<path>&host=<host>&port=<port>&user=<user>&type=ssh&deploy=false
	params := url.Values{}
	params.Set("idePath", o.getDirectory(path.Join("/", "home", o.userName)))
	params.Set("projectPath", workspaceFolder)
	params.Set("host", workspaceID+".devpod")
	params.Set("port", "22") // Standard SSH port - TODO: make configurable
	params.Set("user", o.userName)
	params.Set("type", "ssh")
	params.Set("deploy", "false") // Do not auto-deploy IDE backend

	gatewayURL := "jetbrains-gateway://connect#" + params.Encode()
	o.log.Infof("opening gateway URL %s", gatewayURL)

	err := browser.OpenURL(gatewayURL)
	if err != nil {
		o.log.Errorf("error opening jetbrains-gateway with browser %v", err)

		if runtime.GOOS == "linux" {
			o.log.Debugf("falling back to xdg-open on Linux")
			out, execErr := exec.Command("xdg-open", gatewayURL).CombinedOutput()
			if execErr != nil {
				o.log.Errorf("error opening jetbrains-gateway with xdg-open %v", execErr)
				o.log.Errorf("xdg-open output %s", string(out))
				err = execErr
			} else {
				err = nil // xdg-open succeeded
			}
		}
	}

	if err != nil {
		o.log.Errorf("Failed to open JetBrains Gateway. Ensure JetBrains Gateway is installed. Download from https://www.jetbrains.com/remote-development/gateway/")
		return err
	}
	return nil
}

func (o *GenericJetBrainsServer) GetVolume() string {
	return fmt.Sprintf("type=volume,src=devpod-%s,dst=%s", o.options.ID, o.getDownloadFolder())
}

func (o *GenericJetBrainsServer) getDownloadFolder() string {
	return fmt.Sprintf("/var/devpod/%s", o.options.ID)
}

func (o *GenericJetBrainsServer) Install() error {
	o.log.Debugf("Setup %s...", o.options.DisplayName)
	baseFolder, err := getBaseFolder(o.userName)
	if err != nil {
		return err
	}
	targetLocation := o.getDirectory(baseFolder)

	_, err = os.Stat(targetLocation)
	if err == nil {
		o.log.Debugf("Goland already installed skip install")
		return nil
	}

	o.log.Debugf("Download %s archive", o.options.DisplayName)
	archivePath, err := o.download(o.getDownloadFolder(), o.log)
	if err != nil {
		return err
	}

	o.log.Infof("Extract %s...", o.options.DisplayName)
	err = o.extractArchive(archivePath, targetLocation)
	if err != nil {
		return err
	}

	err = copy2.ChownR(path.Join(baseFolder, ".cache"), o.userName)
	if err != nil {
		return fmt.Errorf("chown %w", err)
	}
	o.log.Infof("Successfully installed %s backend", o.options.DisplayName)
	return nil
}

func getBaseFolder(userName string) (string, error) {
	var err error
	homeFolder := ""
	if userName != "" {
		homeFolder, err = command.GetHome(userName)
	} else {
		homeFolder, err = util.UserHomeDir()
	}
	if err != nil {
		return "", err
	}

	return homeFolder, nil
}

func (o *GenericJetBrainsServer) getDirectory(baseFolder string) string {
	return path.Join(baseFolder, ".cache", "JetBrains", "RemoteDev", "dist", o.options.ID)
}

func (o *GenericJetBrainsServer) extractArchive(fromPath string, toPath string) error {
	file, err := os.Open(fromPath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	return extract.Extract(file, toPath, extract.StripLevels(1))
}

func (o *GenericJetBrainsServer) download(targetFolder string, log log.Logger) (string, error) {
	err := os.MkdirAll(targetFolder, os.ModePerm)
	if err != nil {
		return "", err
	}

	downloadURL := o.options.DownloadAmd64
	if runtime.GOARCH == "arm64" {
		downloadURL = o.options.DownloadArm64
	}

	targetPath := path.Join(filepath.ToSlash(targetFolder), o.options.ID+".tar.gz")

	// initiate download
	log.Infof("Download %s from %s", o.options.DisplayName, downloadURL)
	defer log.Debugf("Successfully downloaded %s", o.options.DisplayName)
	resp, err := devpodhttp.GetHTTPClient().Get(downloadURL)
	if err != nil {
		return "", fmt.Errorf("download binary %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("download binary returned status code %d %w", resp.StatusCode, err)
	}

	stat, err := os.Stat(targetPath)
	if err == nil && stat.Size() == resp.ContentLength {
		return targetPath, nil
	}

	file, err := os.Create(targetPath)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	_, err = io.Copy(file, &ide.ProgressReader{
		Reader:    resp.Body,
		TotalSize: resp.ContentLength,
		Log:       log,
	})
	if err != nil {
		return "", fmt.Errorf("download file %w", err)
	}

	return targetPath, nil
}
