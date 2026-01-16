package vscode

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/pkg/command"
	"github.com/skevetter/devpod/pkg/config"
	copy2 "github.com/skevetter/devpod/pkg/copy"
	"github.com/skevetter/devpod/pkg/ide"
	"github.com/skevetter/devpod/pkg/util"
	"github.com/skevetter/log"
)

const (
	OpenNewWindow = "OPEN_NEW_WINDOW"
)

type Flavor string

const (
	FlavorStable      Flavor = "stable"
	FlavorInsiders    Flavor = "insiders"
	FlavorCursor      Flavor = "cursor"
	FlavorPositron    Flavor = "positron"
	FlavorCodium      Flavor = "codium"
	FlavorWindsurf    Flavor = "windsurf"
	FlavorAntigravity Flavor = "antigravity"
)

func (f Flavor) DisplayName() string {
	switch f {
	case FlavorStable:
		return "VSCode"
	case FlavorInsiders:
		return "VSCode Insiders"
	case FlavorCursor:
		return "Cursor"
	case FlavorPositron:
		return "positron"
	case FlavorCodium:
		return "VSCodium"
	case FlavorWindsurf:
		return "Windsurf"
	case FlavorAntigravity:
		return "Antigravity"
	default:
		return "VSCode"
	}
}

var Options = ide.Options{
	OpenNewWindow: {
		Name:        OpenNewWindow,
		Description: "If true, DevPod will open the project in a new window",
		Default:     "true",
		Enum: []string{
			"false",
			"true",
		},
	},
}

func NewVSCodeServer(extensions []string, settings string, userName string, values map[string]config.OptionValue, flavor Flavor, log log.Logger) *VsCodeServer {
	if flavor == "" {
		flavor = FlavorStable
	}

	return &VsCodeServer{
		values:     values,
		extensions: extensions,
		settings:   settings,
		userName:   userName,
		log:        log,
		flavor:     flavor,
	}
}

type VsCodeServer struct {
	values     map[string]config.OptionValue
	extensions []string
	settings   string
	userName   string
	flavor     Flavor
	log        log.Logger
}

func (o *VsCodeServer) InstallExtensions() error {
	location, err := prepareServerLocation(o.userName, false, o.flavor)
	if err != nil {
		return err
	}

	binPath := o.waitForServerBinary(location)
	if binPath == "" {
		return fmt.Errorf("unable to locate server binary in workspace")
	}
	// start log writer
	writer := o.log.Writer(logrus.InfoLevel, false)
	errwriter := o.log.Writer(logrus.ErrorLevel, false)
	defer func() { _ = writer.Close() }()
	defer func() { _ = errwriter.Close() }()

	// download extensions
	for _, extension := range o.extensions {
		o.log.Info("install extension " + extension)
		runCommand := fmt.Sprintf("%s serve-local --accept-server-license-terms --install-extension '%s'", binPath, extension)
		args := []string{}
		if o.userName != "" {
			args = append(args, "su", o.userName, "-c", runCommand)
		} else {
			args = append(args, "sh", "-c", runCommand)
		}
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdout = writer
		cmd.Stderr = errwriter
		err := cmd.Run()
		if err != nil {
			o.log.WithFields(logrus.Fields{
				"extension": extension,
				"error":     err,
			}).Warn("failed installing extension")
		}
		o.log.WithFields(logrus.Fields{
			"extension": extension,
		}).Info("installed extension")
	}

	return nil
}

func (o *VsCodeServer) Install() error {
	location, err := prepareServerLocation(o.userName, true, o.flavor)
	if err != nil {
		return err
	}

	settingsDir := filepath.Join(location, "data", "Machine")
	err = os.MkdirAll(settingsDir, 0755)
	if err != nil {
		return err
	}

	// is installed
	settingsFile := filepath.Join(settingsDir, "settings.json")
	_, err = os.Stat(settingsFile)
	if err == nil {
		return nil
	}

	InstallAPKRequirements(o.log)

	// add settings
	if o.settings == "" {
		o.settings = "{}"
	}

	// set settings
	err = os.WriteFile(settingsFile, []byte(o.settings), 0600)
	if err != nil {
		return err
	}

	// chown location
	if o.userName != "" {
		err = copy2.ChownR(location, o.userName)
		if err != nil {
			return fmt.Errorf("chown %w", err)
		}
	}

	return nil
}

const (
	serverSearchTimeout   = time.Minute * 10
	initialBackoff        = time.Second * 2
	maxBackoff            = time.Second * 30
	binaryValidateTimeout = time.Second * 4
	maxSearchDepth        = 10
)

type serverConfig struct {
	serverName string
	binName    string
}

func (o *VsCodeServer) getServerConfig() serverConfig {
	switch o.flavor {
	case FlavorStable:
		return serverConfig{"vscode-server", "code-server"}
	case FlavorCursor:
		return serverConfig{"cursor-server", "cursor-server"}
	case FlavorPositron:
		return serverConfig{"positron-server", "positron-server"}
	case FlavorCodium:
		return serverConfig{"vscodium-server", "codium-server"}
	case FlavorInsiders:
		return serverConfig{"vscode-server-insiders", "code-server-insiders"}
	default:
		return serverConfig{"vscode-server", "code-server"}
	}
}

func (o *VsCodeServer) findServerBinaryPath(location string) string {
	cfg := o.getServerConfig()

	searches := []struct {
		name string
		find func() string
	}{
		{"system PATH", func() string { return o.findInSystemPath(cfg.binName) }},
		{"install dir", func() string { return o.findBinaryInDir(location, cfg.binName) }},
	}

	for _, s := range searches {
		if path := s.find(); path != "" {
			if o.validateBinary(path) {
				o.log.WithFields(logrus.Fields{"server": cfg.serverName, "path": path, "location": s.name}).Info("found server binary")
				return path
			}
		}
	}

	return ""
}

func (o *VsCodeServer) waitForServerBinary(location string) string {
	deadline := time.Now().Add(serverSearchTimeout)
	backoff := initialBackoff
	attempts := 0

	for time.Now().Before(deadline) {
		if path := o.findServerBinaryPath(location); path != "" {
			return path
		}

		if attempts == 0 || attempts%10 == 0 {
			o.log.WithFields(logrus.Fields{"attempts": attempts}).Debug("waiting for server installation")
		}

		time.Sleep(backoff)
		backoff = min(backoff*2, maxBackoff)
		attempts++
	}

	o.log.WithFields(logrus.Fields{"attempts": attempts}).Warn("timed out waiting for server")
	return ""
}

func (o *VsCodeServer) findInSystemPath(binName string) string {
	if path, err := exec.LookPath(binName); err == nil {
		if o.validateBinary(path) {
			return path
		}
	}
	return ""
}

func (o *VsCodeServer) findBinaryInDir(root, binName string) string {
	var found string
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || found != "" {
			return filepath.SkipDir
		}

		depth := strings.Count(strings.TrimPrefix(path, root), string(filepath.Separator))
		if depth > maxSearchDepth {
			return filepath.SkipDir
		}

		if !d.IsDir() && d.Name() == binName {
			found = path
			return filepath.SkipAll
		}

		return nil
	})
	return found
}

func (o *VsCodeServer) validateBinary(binPath string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), binaryValidateTimeout)
	defer cancel()
	return exec.CommandContext(ctx, binPath, "--help").Run() == nil
}

func prepareServerLocation(userName string, create bool, flavor Flavor) (string, error) {
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

	folderName := ".vscode-server"
	switch flavor {
	case FlavorStable:
		folderName = ".vscode-server"
	case FlavorInsiders:
		folderName = ".vscode-server-insiders"
	case FlavorCursor:
		folderName = ".cursor-server"
	case FlavorPositron:
		folderName = ".positron-server"
	case FlavorCodium:
		folderName = ".vscodium-server"
	case FlavorWindsurf:
		folderName = ".windsurf-server"
	}

	folder := filepath.Join(homeFolder, folderName)
	if create {
		err = os.MkdirAll(folder, 0755)
		if err != nil {
			return "", err
		}
	}

	return folder, nil
}
