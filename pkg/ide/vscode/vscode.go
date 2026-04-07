package vscode

import (
	"bytes"
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
	FlavorBob         Flavor = "bob"
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

// Map of VS Code flavors. The correct configuration options can be found in
// `resources/app/product.json` of the VS Code fork.
//
// - Display name
// - Server directory, matching serverDataFolderName in `product.json`.
// - Server binary name, matching serverApplicationName in `product.json`.
var flavorConfigs = map[Flavor]flavorConfig{
	FlavorStable:      {"VS Code", ".vscode-server", "code-server"},
	FlavorInsiders:    {"VS Code Insiders", ".vscode-server-insiders", "code-server-insiders"},
	FlavorCursor:      {"Cursor", ".cursor-server", "cursor-server"},
	FlavorPositron:    {"positron", ".positron-server", "positron-server"},
	FlavorCodium:      {"VSCodium", ".vscodium-server", "codium-server"},
	FlavorWindsurf:    {"Windsurf", ".windsurf-server", "windsurf-server"},
	FlavorAntigravity: {"Antigravity", ".antigravity-server", "antigravity-server"},
	FlavorBob:         {"Bob", ".bobide-server", "bobide-server"},
}

func (f Flavor) DisplayName() string {
	if cfg, ok := flavorConfigs[f]; ok {
		return cfg.displayName
	}
	return "VS Code"
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
			o.log.WithFields(logrus.Fields{"extension": ext, "error": err}).
				Warn("failed installing extension")
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
	// #nosec G301 -- TODO Consider using a more secure permission setting and ownership if needed.
	if err := os.MkdirAll(filepath.Dir(settingsFile), 0o755); err != nil {
		return err
	}

	InstallAPKRequirements(o.log)

	settings := o.settings
	if settings == "" {
		settings = "{}"
	}

	return os.WriteFile(settingsFile, []byte(settings), 0o600)
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
	args := []string{"--install-extension", extension}

	if o.userName != "" {
		cmd := shellquote.Join(append([]string{binPath}, args...)...)
		return exec.Command("su", o.userName, "-c", cmd)
	}
	return exec.Command(binPath, args...)
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
		{"running process", func() string { return o.findRunningServer(cfg.binName) }},
		{"install dir", func() string { return o.findInDir(location, cfg.binName) }},
		{"system PATH", func() string { return o.findInSystemPath(cfg.binName) }},
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
			o.log.WithFields(logrus.Fields{"attempts": attempts}).
				Debug("waiting for server installation")
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

// findRunningServer looks for a running process whose command line contains
// the server binary name, then extracts the executable path. It reads
// /proc/*/cmdline directly to avoid a dependency on pgrep/procps.
func (o *VsCodeServer) findRunningServer(binName string) string {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		o.log.Debugf("cannot read /proc, skipping process discovery: %v", err)
		return ""
	}

	for _, entry := range entries {
		if !entry.IsDir() || !isNumeric(entry.Name()) {
			continue
		}

		cmdline, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if err != nil {
			continue
		}

		path := matchServerProcess(cmdline, binName)
		if path != "" {
			o.log.Debugf("found running server process (pid=%s, path=%s)", entry.Name(), path)
			return path
		}
	}

	return ""
}

// matchServerProcess checks whether a /proc cmdline (NUL-delimited) belongs
// to a VS Code server process and returns the binary path if it does.
func matchServerProcess(cmdline []byte, binName string) string {
	if len(cmdline) == 0 {
		return ""
	}

	args := bytes.Split(cmdline, []byte{0})
	if len(args) == 0 {
		return ""
	}

	// The server binary is a shell wrapper that starts node, so the cmdline
	// typically looks like: /path/to/.../bin/code-server --host ...
	// But it could also be invoked by sh: /bin/sh /path/to/code-server ...
	for _, arg := range args {
		s := string(arg)
		if filepath.Base(s) == binName {
			// Resolve to absolute path if possible
			if abs, err := filepath.Abs(s); err == nil {
				s = abs
			}
			return s
		}
	}

	return ""
}

func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

// findInDir walks root looking for files named binName, collecting all
// candidates. When multiple matches exist (e.g. several VS Code server
// versions), it returns the one whose parent directory has the most recent
// modification time, so that an auto-updated server is preferred over a
// stale one.
func (o *VsCodeServer) findInDir(root, binName string) string {
	type candidate struct {
		path    string
		modTime time.Time
	}
	var candidates []candidate

	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return filepath.SkipDir
		}

		pathRelative := strings.TrimPrefix(path, root)
		depth := strings.Count(pathRelative, string(filepath.Separator))
		if depth > maxSearchDepth {
			return filepath.SkipDir
		}

		// The VS Code server gets installed into a staging directory, which is
		// later renamed. Do not consider the staging directory a valid
		// destination, as it won't be valid by the time the server binary is
		// called.
		if d.IsDir() && strings.Contains(pathRelative, ".staging") {
			return filepath.SkipDir
		}

		if !d.IsDir() && d.Name() == binName {
			dirInfo, err := os.Stat(filepath.Dir(path))
			if err != nil {
				return nil
			}
			candidates = append(candidates, candidate{path: path, modTime: dirInfo.ModTime()})
		}

		return nil
	})

	if len(candidates) == 0 {
		return ""
	}

	// Return the candidate with the newest parent directory mtime.
	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.modTime.After(best.modTime) {
			best = c
		}
	}

	if len(candidates) > 1 {
		o.log.Debugf(
			"multiple server binaries found (count=%d), chose newest: %s",
			len(candidates),
			best.path,
		)
	}

	return best.path
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

	if mode.Perm()&0o002 != 0 {
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
		// #nosec G301 -- TODO Consider using a more secure permission setting and ownership if needed.
		if err := os.MkdirAll(folder, 0o755); err != nil {
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
