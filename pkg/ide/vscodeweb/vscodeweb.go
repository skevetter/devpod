package vscodeweb

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/pkg/command"
	"github.com/skevetter/devpod/pkg/config"
	copy2 "github.com/skevetter/devpod/pkg/copy"
	"github.com/skevetter/devpod/pkg/extract"
	devpodhttp "github.com/skevetter/devpod/pkg/http"
	"github.com/skevetter/devpod/pkg/ide"
	"github.com/skevetter/devpod/pkg/ide/vscode"
	"github.com/skevetter/devpod/pkg/util"
	"github.com/skevetter/log"
)

const (
	// The VS Code CLI ("code" binary) is a small static musl build that runs
	// "code serve-web". The %s is the build channel (stable | insider).
	DownloadAmd64Template = "https://code.visualstudio.com/sha/download?build=%s&os=cli-alpine-x64"
	DownloadArm64Template = "https://code.visualstudio.com/sha/download?build=%s&os=cli-alpine-arm64"
)

const (
	ForwardPortsOption  = "FORWARD_PORTS"
	OpenOption          = "OPEN"
	BindAddressOption   = "BIND_ADDRESS"
	VersionOption       = "VERSION"
	DownloadAmd64Option = "DOWNLOAD_AMD64"
	DownloadArm64Option = "DOWNLOAD_ARM64"
)

var Options = ide.Options{
	ForwardPortsOption: {
		Name:        ForwardPortsOption,
		Description: "If DevPod should automatically do port-forwarding",
		Default:     "true",
		Enum:        []string{"true", "false"},
	},
	BindAddressOption: {
		Name:        BindAddressOption,
		Description: "The address to bind VS Code Web to locally, e.g. 0.0.0.0:12345",
		Default:     "",
	},
	VersionOption: {
		Name:        VersionOption,
		Description: "The VS Code CLI build channel to download (stable or insider)",
		Default:     "stable",
	},
	OpenOption: {
		Name:        OpenOption,
		Description: "If DevPod should automatically open the browser",
		Default:     "true",
		Enum:        []string{"true", "false"},
	},
	DownloadArm64Option: {
		Name:        DownloadArm64Option,
		Description: "The download url for the arm64 VS Code CLI binary",
	},
	DownloadAmd64Option: {
		Name:        DownloadAmd64Option,
		Description: "The download url for the amd64 VS Code CLI binary",
	},
}

const DefaultVSCodePort = 10800

type VSCodeWebServer struct {
	values     map[string]config.OptionValue
	extensions []string
	settings   string
	userName   string
	host       string
	port       string
	log        log.Logger
}

//nolint:revive
func NewVSCodeWebServer(
	extensions []string,
	settings string,
	userName string,
	host, port string,
	values map[string]config.OptionValue,
	log log.Logger,
) *VSCodeWebServer {
	return &VSCodeWebServer{
		values:     values,
		extensions: extensions,
		settings:   settings,
		userName:   userName,
		host:       host,
		port:       port,
		log:        log,
	}
}

func (o *VSCodeWebServer) InstallExtensions() error {
	if err := o.installExtensions(); err != nil {
		return fmt.Errorf("install extensions: %w", err)
	}
	return nil
}

func (o *VSCodeWebServer) Install() error {
	location, err := prepareVSCodeWebLocation(o.userName)
	if err != nil {
		return err
	}

	// already installed?
	if _, err = os.Stat(filepath.Join(location, "code")); err == nil {
		return nil
	}

	url := o.getReleaseUrl()

	vscode.InstallAPKRequirements(o.log)

	resp, err := devpodhttp.GetHTTPClient().Get(url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	// The CLI tarball is a single flat "code" file; StripLevels(1) keeps it
	// (the strip loop stops on single-component names).
	if err = extract.Extract(resp.Body, location, extract.StripLevels(1)); err != nil {
		return fmt.Errorf("extract vscode cli: %w", err)
	}

	if o.userName != "" {
		if err = copy2.ChownR(location, o.userName); err != nil {
			return fmt.Errorf("chown: %w", err)
		}
	}

	if err = o.installSettings(); err != nil {
		return fmt.Errorf("install settings: %w", err)
	}

	return nil
}

func (o *VSCodeWebServer) Start() error {
	location, err := prepareVSCodeWebLocation(o.userName)
	if err != nil {
		return err
	}

	if o.host == "" {
		o.host = "0.0.0.0"
	}
	if o.port == "" {
		o.port = strconv.Itoa(DefaultVSCodePort)
	}

	binaryPath := filepath.Join(location, "code")
	if _, err = os.Stat(binaryPath); err != nil {
		return fmt.Errorf("find binary: %w", err)
	}

	return command.StartBackgroundOnce("vscode-web", func() (*exec.Cmd, error) {
		o.log.Infof("Starting vscode-web in background...")
		runCommand := fmt.Sprintf(
			"%s serve-web --accept-server-license-terms --without-connection-token "+
				"--host '%s' --port '%s' --server-data-dir '%s'",
			binaryPath, o.host, o.port, location,
		)
		args := []string{}
		if o.userName != "" {
			args = append(args, "su", o.userName, "-c", runCommand)
		} else {
			args = append(args, "sh", "-c", runCommand)
		}
		// #nosec G204 -- args constructed from trusted internal values
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = location
		return cmd, nil
	})
}

func (o *VSCodeWebServer) getReleaseUrl() string {
	var url string
	version := Options.GetValue(o.values, VersionOption)

	if runtime.GOARCH == "arm64" {
		url = Options.GetValue(o.values, DownloadArm64Option)
		if url == "" {
			url = fmt.Sprintf(DownloadArm64Template, version)
		}
	} else {
		url = Options.GetValue(o.values, DownloadAmd64Option)
		if url == "" {
			url = fmt.Sprintf(DownloadAmd64Template, version)
		}
	}

	return url
}

func (o *VSCodeWebServer) installExtensions() error {
	if len(o.extensions) == 0 {
		return nil
	}

	location, err := prepareVSCodeWebLocation(o.userName)
	if err != nil {
		return err
	}

	out := o.log.Writer(logrus.InfoLevel, false)
	defer func() { _ = out.Close() }()

	binaryPath := filepath.Join(location, "code")
	extensionsDir := filepath.Join(location, "extensions")
	for _, extension := range o.extensions {
		o.log.Info("Install extension " + extension + "...")
		runCommand := fmt.Sprintf(
			"%s --install-extension '%s' --extensions-dir '%s'",
			binaryPath, extension, extensionsDir,
		)
		args := []string{}
		if o.userName != "" {
			args = append(args, "su", o.userName, "-c", runCommand)
		} else {
			args = append(args, "sh", "-c", runCommand)
		}
		// #nosec G204 -- args constructed from trusted internal values
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdout = out
		cmd.Stderr = out
		if err = cmd.Run(); err != nil {
			o.log.Errorf("failed installing extension: extension=%s, error=%v", extension, err)
		} else {
			o.log.Infof("installed extension: extension=%s", extension)
		}
	}

	return nil
}

func (o *VSCodeWebServer) installSettings() error {
	if len(o.settings) == 0 {
		return nil
	}

	location, err := prepareVSCodeWebLocation(o.userName)
	if err != nil {
		return err
	}

	settingsDir := filepath.Join(location, "data", "Machine")
	// #nosec G301
	if err = os.MkdirAll(settingsDir, 0o755); err != nil {
		return err
	}

	if err = os.WriteFile(
		filepath.Join(settingsDir, "settings.json"),
		[]byte(o.settings),
		0o600,
	); err != nil {
		return err
	}

	return copy2.ChownR(location, o.userName)
}

func prepareVSCodeWebLocation(userName string) (string, error) {
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

	folder := filepath.Join(homeFolder, ".vscode-web")
	// #nosec G301
	if err = os.MkdirAll(folder, 0o755); err != nil {
		return "", err
	}

	return folder, nil
}
