package vscode

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kballard/go-shellquote"
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

const (
	serverSearchTimeout   = time.Minute * 10
	initialBackoff        = time.Second * 2
	maxBackoff            = time.Second * 30
	binaryValidateTimeout = time.Second * 4
	maxSearchDepth        = 10
)

type flavorConfig struct {
	displayName string
	serverDir   string
	binName     string
}

var flavorConfigs = map[Flavor]flavorConfig{
	FlavorStable:      {"VSCode", ".vscode-server", "code-server"},
	FlavorInsiders:    {"VSCode Insiders", ".vscode-server-insiders", "code-server-insiders"},
	FlavorCursor:      {"Cursor", ".cursor-server", "cursor"},
	FlavorPositron:    {"positron", ".positron-server", "positron"},
	FlavorCodium:      {"VSCodium", ".vscodium-server", "codium"},
	FlavorWindsurf:    {"Windsurf", ".windsurf-server", "windsurf"},
	FlavorAntigravity: {"Antigravity", ".antigravity-server", "agy"},
}

func (f Flavor) DisplayName() string {
	if cfg, ok := flavorConfigs[f]; ok {
		return cfg.displayName
	}
	return "VSCode"
}

type ServerOptions struct {
	Extensions []string
	Settings   string
	UserName   string
	Values     map[string]config.OptionValue
	Flavor     Flavor
	Log        log.Logger
}

type VsCodeServer struct {
	values     map[string]config.OptionValue
	extensions []string
	settings   string
	userName   string
	flavor     Flavor
	log        log.Logger
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

func NewVSCodeServer(opts ServerOptions) *VsCodeServer {
	if opts.Flavor == "" {
		opts.Flavor = FlavorStable
	}

	return &VsCodeServer{
		values:     opts.Values,
		extensions: opts.Extensions,
		settings:   opts.Settings,
		userName:   opts.UserName,
		log:        opts.Log,
		flavor:     opts.Flavor,
	}
}

func (o *VsCodeServer) InstallExtensions() error {
	location, err := o.prepareServerLocation(false)
	if err != nil {
		return err
	}

	binPath := o.waitForServerBinary(location)
	if binPath == "" {
		return fmt.Errorf("unable to locate server binary")
	}

	writer := o.log.Writer(logrus.InfoLevel, false)
	errWriter := o.log.Writer(logrus.ErrorLevel, false)
	defer func() { _ = writer.Close() }()
	defer func() { _ = errWriter.Close() }()

	for _, ext := range o.extensions {
		if err := o.installExtension(binPath, ext, writer, errWriter); err != nil {
			o.log.WithFields(logrus.Fields{"extension": ext, "error": err}).Warn("failed installing extension")
		} else {
			o.log.WithFields(logrus.Fields{"extension": ext}).Info("installed extension")
		}
	}

	return nil
}

func (o *VsCodeServer) Install() error {
	location, err := o.prepareServerLocation(true)
	if err != nil {
		return err
	}

	settingsFile := filepath.Join(location, "data", "Machine", "settings.json")
	if o.isAlreadyInstalled(settingsFile) {
		return nil
	}

	if err := o.setupSettings(settingsFile); err != nil {
		return err
	}

	if o.userName != "" {
		return o.changeOwnership(location)
	}

	return nil
}

func (o *VsCodeServer) isAlreadyInstalled(settingsFile string) bool {
	_, err := os.Stat(settingsFile)
	return err == nil
}

func (o *VsCodeServer) setupSettings(settingsFile string) error {
	if err := os.MkdirAll(filepath.Dir(settingsFile), 0755); err != nil {
		return err
	}

	InstallAPKRequirements(o.log)

	settings := o.settings
	if settings == "" {
		settings = "{}"
	}

	return os.WriteFile(settingsFile, []byte(settings), 0600)
}

func (o *VsCodeServer) changeOwnership(location string) error {
	if err := copy2.ChownR(location, o.userName); err != nil {
		return fmt.Errorf("chown: %w", err)
	}
	return nil
}

func (o *VsCodeServer) installExtension(binPath, extension string, stdout, stderr io.Writer) error {
	o.log.WithFields(logrus.Fields{"extension": extension}).Info("installing extension")

	cmd := o.buildExtensionCommand(binPath, extension)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func (o *VsCodeServer) buildExtensionCommand(binPath, extension string) *exec.Cmd {
	if o.userName != "" {
		cmd := shellquote.Join(
			binPath,
			"serve-local",
			"--accept-server-license-terms",
			"--install-extension",
			extension,
		)
		return exec.Command("su", o.userName, "-c", cmd)
	}
	return exec.Command(binPath, "serve-local", "--accept-server-license-terms", "--install-extension", extension)
}

func (o *VsCodeServer) findServerBinaryPath(location string) string {
	cfg, ok := flavorConfigs[o.flavor]
	if !ok {
		cfg = flavorConfigs[FlavorStable]
	}

	searches := []struct {
		name string
		fn   func() string
	}{
		{"system PATH", func() string { return o.findInSystemPath(cfg.binName) }},
		{"install dir", func() string { return o.findInDir(location, cfg.binName) }},
	}

	for _, s := range searches {
		if path := s.fn(); path != "" && o.validateBinary(path) {
			o.log.WithFields(logrus.Fields{
				"server":   cfg.serverDir,
				"path":     path,
				"location": s.name,
			}).Info("found server binary")
			return path
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
	path, err := exec.LookPath(binName)
	if err != nil {
		return ""
	}
	return path
}

func (o *VsCodeServer) findInDir(root, binName string) string {
	var found string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
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
	if !o.isSafeBinary(binPath) {
		o.log.WithFields(logrus.Fields{"path": binPath}).Warn("binary failed safety checks")
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), binaryValidateTimeout)
	defer cancel()
	return exec.CommandContext(ctx, binPath, "--help").Run() == nil
}

func (o *VsCodeServer) isSafeBinary(binPath string) bool {
	info, err := os.Stat(binPath)
	if err != nil {
		return false
	}

	mode := info.Mode()
	if !mode.IsRegular() {
		return false
	}

	if mode.Perm()&0002 != 0 {
		return false
	}

	return true
}

func (o *VsCodeServer) prepareServerLocation(create bool) (string, error) {
	homeFolder, err := o.getHomeFolder()
	if err != nil {
		return "", err
	}

	cfg, ok := flavorConfigs[o.flavor]
	if !ok {
		cfg = flavorConfigs[FlavorStable]
	}

	folder := filepath.Join(homeFolder, cfg.serverDir)
	if create {
		if err := os.MkdirAll(folder, 0755); err != nil {
			return "", err
		}
	}

	return folder, nil
}

func (o *VsCodeServer) getHomeFolder() (string, error) {
	if o.userName != "" {
		return command.GetHome(o.userName)
	}
	return util.UserHomeDir()
}
