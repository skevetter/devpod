package codeserver

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
	// code-server is Coder's standalone VS Code in the browser. The tarball is
	// self-contained (no runtime download). %s is the version, e.g. 4.126.0.
	DownloadAmd64Template = "https://github.com/coder/code-server/releases/download/v%s/code-server-%s-linux-amd64.tar.gz"
	DownloadArm64Template = "https://github.com/coder/code-server/releases/download/v%s/code-server-%s-linux-arm64.tar.gz"
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
		Description: "The address to bind code-server to locally, e.g. 0.0.0.0:12345",
		Default:     "",
	},
	VersionOption: {
		Name:        VersionOption,
		Description: "The version for the code-server binary",
		Default:     "4.126.0",
	},
	OpenOption: {
		Name:        OpenOption,
		Description: "If DevPod should automatically open the browser",
		Default:     "true",
		Enum:        []string{"true", "false"},
	},
	DownloadArm64Option: {
		Name:        DownloadArm64Option,
		Description: "The download url for the arm64 code-server binary",
	},
	DownloadAmd64Option: {
		Name:        DownloadAmd64Option,
		Description: "The download url for the amd64 code-server binary",
	},
}

const DefaultVSCodePort = 10800

type CodeServerServer struct {
	values     map[string]config.OptionValue
	extensions []string
	settings   string
	userName   string
	host       string
	port       string
	log        log.Logger
}

//nolint:revive
func NewCodeServerServer(
	extensions []string,
	settings string,
	userName string,
	host, port string,
	values map[string]config.OptionValue,
	log log.Logger,
) *CodeServerServer {
	return &CodeServerServer{
		values:     values,
		extensions: extensions,
		settings:   settings,
		userName:   userName,
		host:       host,
		port:       port,
		log:        log,
	}
}

func (o *CodeServerServer) InstallExtensions() error {
	if err := o.installExtensions(); err != nil {
		return fmt.Errorf("install extensions: %w", err)
	}
	return nil
}

func (o *CodeServerServer) Install() error {
	location, err := prepareCodeServerLocation(o.userName)
	if err != nil {
		return err
	}

	// already installed?
	if _, err = os.Stat(filepath.Join(location, "bin", "code-server")); err == nil {
		return nil
	}

	url := o.getReleaseUrl()

	vscode.InstallAPKRequirements(o.log)

	resp, err := devpodhttp.GetHTTPClient().Get(url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	// Tarball top dir is code-server-<ver>-linux-<arch>/; StripLevels(1) yields bin/code-server.
	if err = extract.Extract(resp.Body, location, extract.StripLevels(1)); err != nil {
		return fmt.Errorf("extract code-server: %w", err)
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

func (o *CodeServerServer) Start() error {
	location, err := prepareCodeServerLocation(o.userName)
	if err != nil {
		return err
	}

	if o.host == "" {
		o.host = "0.0.0.0"
	}
	if o.port == "" {
		o.port = strconv.Itoa(DefaultVSCodePort)
	}

	binaryPath := filepath.Join(location, "bin", "code-server")
	if _, err = os.Stat(binaryPath); err != nil {
		return fmt.Errorf("find binary: %w", err)
	}

	dataDir := filepath.Join(location, "data")
	extensionsDir := filepath.Join(location, "extensions")
	return command.StartBackgroundOnce("code-server", func() (*exec.Cmd, error) {
		o.log.Infof("Starting code-server in background...")
		runCommand := fmt.Sprintf(
			"%s --bind-addr '%s:%s' --auth none --user-data-dir '%s' --extensions-dir '%s'",
			binaryPath, o.host, o.port, dataDir, extensionsDir,
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

func (o *CodeServerServer) getReleaseUrl() string {
	var url string
	version := Options.GetValue(o.values, VersionOption)

	if runtime.GOARCH == "arm64" {
		url = Options.GetValue(o.values, DownloadArm64Option)
		if url == "" {
			url = fmt.Sprintf(DownloadArm64Template, version, version)
		}
	} else {
		url = Options.GetValue(o.values, DownloadAmd64Option)
		if url == "" {
			url = fmt.Sprintf(DownloadAmd64Template, version, version)
		}
	}

	return url
}

func (o *CodeServerServer) installExtensions() error {
	if len(o.extensions) == 0 {
		return nil
	}

	location, err := prepareCodeServerLocation(o.userName)
	if err != nil {
		return err
	}

	out := o.log.Writer(logrus.InfoLevel, false)
	defer func() { _ = out.Close() }()

	binaryPath := filepath.Join(location, "bin", "code-server")
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

func (o *CodeServerServer) installSettings() error {
	if len(o.settings) == 0 {
		return nil
	}

	location, err := prepareCodeServerLocation(o.userName)
	if err != nil {
		return err
	}

	// code-server reads user settings from <user-data-dir>/User/settings.json.
	settingsDir := filepath.Join(location, "data", "User")
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

func prepareCodeServerLocation(userName string) (string, error) {
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

	folder := filepath.Join(homeFolder, ".code-server")
	// #nosec G301
	if err = os.MkdirAll(folder, 0o755); err != nil {
		return "", err
	}

	return folder, nil
}
