package ssh

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/skevetter/devpod/pkg/util"
	"github.com/skevetter/log"
	"github.com/skevetter/log/scanner"
)

var configLock sync.Mutex

var (
	MarkerStartPrefix = "# DevPod Start "
	MarkerEndPrefix   = "# DevPod End "
)

type SSHConfigParams struct {
	SSHConfigPath        string
	SSHConfigIncludePath string
	Context              string
	Workspace            string
	User                 string
	Workdir              string
	Command              string
	GPGAgent             bool
	DevPodHome           string
	Provider             string
	Log                  log.Logger
}

func ConfigureSSHConfig(params SSHConfigParams) error {
	configLock.Lock()
	defer configLock.Unlock()

	targetPath := params.SSHConfigPath
	if params.SSHConfigIncludePath != "" {
		targetPath = params.SSHConfigIncludePath
	}

	newFile, err := addHost(addHostParams{
		path:       targetPath,
		host:       params.Workspace + "." + "devpod",
		user:       params.User,
		context:    params.Context,
		workspace:  params.Workspace,
		workdir:    params.Workdir,
		command:    params.Command,
		gpgagent:   params.GPGAgent,
		devPodHome: params.DevPodHome,
		provider:   params.Provider,
	})
	if err != nil {
		return fmt.Errorf("parse ssh config: %w", err)
	}

	return writeSSHConfig(targetPath, newFile, params.Log)
}

type DevPodSSHEntry struct {
	Host      string
	User      string
	Workspace string
}

type addHostParams struct {
	path       string
	host       string
	user       string
	context    string
	workspace  string
	workdir    string
	command    string
	gpgagent   bool
	devPodHome string
	provider   string
}

func addHost(params addHostParams) (string, error) {
	newConfig, err := removeFromConfig(params.path, params.host)
	if err != nil {
		return "", err
	}

	// get path to executable
	execPath, err := os.Executable()
	if err != nil {
		return "", err
	}

	return addHostSection(newConfig, execPath, params)
}

// proxyCommandBuilder builds SSH ProxyCommand strings
type proxyCommandBuilder struct {
	baseCommand string
	options     []string
}

func newProxyCommandBuilder(execPath, context, user, workspace string) *proxyCommandBuilder {
	return &proxyCommandBuilder{
		baseCommand: fmt.Sprintf("\"%s\" ssh --stdio --context %s --user %s %s", execPath, context, user, workspace),
	}
}

func (b *proxyCommandBuilder) withDevPodHome(home string) *proxyCommandBuilder {
	if home != "" {
		b.options = append(b.options, fmt.Sprintf("--devpod-home \"%s\"", home))
	}
	return b
}

func (b *proxyCommandBuilder) withWorkdir(workdir string) *proxyCommandBuilder {
	if workdir != "" {
		b.options = append(b.options, fmt.Sprintf("--workdir \"%s\"", workdir))
	}
	return b
}

func (b *proxyCommandBuilder) withGPGAgent(enabled bool) *proxyCommandBuilder {
	if enabled {
		b.options = append(b.options, "--gpg-agent-forwarding")
	}
	return b
}

func (b *proxyCommandBuilder) build() string {
	if len(b.options) == 0 {
		return "  ProxyCommand " + b.baseCommand
	}
	return fmt.Sprintf("  ProxyCommand %s %s", b.baseCommand, strings.Join(b.options, " "))
}

// sshConfigBuilder builds SSH config entries
type sshConfigBuilder struct {
	lines []string
}

func newSSHConfigBuilder(host string) *sshConfigBuilder {
	return &sshConfigBuilder{
		lines: []string{
			MarkerStartPrefix + host,
			"Host " + host,
		},
	}
}

func (b *sshConfigBuilder) addSSHOptions(provider string) *sshConfigBuilder {
	b.lines = append(b.lines,
		"  ForwardAgent yes",
		"  LogLevel error",
		"  StrictHostKeyChecking no",
		"  UserKnownHostsFile /dev/null",
		"  HostKeyAlgorithms rsa-sha2-256,rsa-sha2-512,ssh-rsa",
	)

	// TODO: Make SSH timeout configurable per provider via provider options
	// The ms-vscode-remote.remote-ssh extension times out after 15s by default
	// This is insufficient for the aws AWS provider as it needs additional time to
	// connect to the instance
	//
	// The SSH config ConnectTimeout overrides the VSCode Remote-SSH remote.SSH.connectTimeout setting
	// https://github.com/microsoft/vscode-remote-release/issues/8519
	if strings.Contains(provider, "aws") {
		b.lines = append(b.lines, "  ConnectTimeout 60")
	}

	return b
}

func (b *sshConfigBuilder) addProxyCommand(proxyCmd string) *sshConfigBuilder {
	b.lines = append(b.lines, proxyCmd)
	return b
}

func (b *sshConfigBuilder) addUser(user, host string) *sshConfigBuilder {
	b.lines = append(b.lines, "  User "+user, MarkerEndPrefix+host)
	return b
}

func (b *sshConfigBuilder) build() []string {
	return b.lines
}

// buildProxyCommand creates the ProxyCommand string
func buildProxyCommand(execPath string, params addHostParams) string {
	if params.command != "" {
		return fmt.Sprintf("  ProxyCommand \"%s\"", params.command)
	}

	return newProxyCommandBuilder(execPath, params.context, params.user, params.workspace).
		withDevPodHome(params.devPodHome).
		withWorkdir(params.workdir).
		withGPGAgent(params.gpgagent).
		build()
}

// buildSSHConfigLines creates the SSH config entry lines
func buildSSHConfigLines(params addHostParams, proxyCmd string) []string {
	return newSSHConfigBuilder(params.host).
		addSSHOptions(params.provider).
		addProxyCommand(proxyCmd).
		addUser(params.user, params.host).
		build()
}

// findInsertPosition finds where to insert new SSH config entry
func findInsertPosition(config string) (int, []string, error) {
	lineNumber := 0
	found := false
	lines := []string{}
	commentLines := 0

	scanner := bufio.NewScanner(strings.NewReader(config))
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(strings.TrimSpace(line), "Host") && !found {
			found = true
			lineNumber = max(lineNumber-commentLines, 0)
		}

		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			commentLines++
		} else {
			commentLines = 0
		}

		if !found {
			lineNumber++
		}

		lines = append(lines, line)
	}

	if err := scanner.Err(); err != nil {
		return 0, nil, err
	}

	return lineNumber, lines, nil
}

// mergeSSHConfig inserts new lines into existing config
func mergeSSHConfig(lines, newLines []string, position int) string {
	merged := slices.Insert(lines, position, newLines...)

	newLineSep := "\n"
	if runtime.GOOS == "windows" {
		newLineSep = "\r\n"
	}

	return strings.Join(merged, newLineSep)
}

func addHostSection(config, execPath string, params addHostParams) (string, error) {
	proxyCmd := buildProxyCommand(execPath, params)
	newLines := buildSSHConfigLines(params, proxyCmd)

	position, lines, err := findInsertPosition(config)
	if err != nil {
		return config, err
	}

	return mergeSSHConfig(lines, newLines, position), nil
}

func GetUser(workspaceID string, sshConfigPath string, sshConfigIncludePath string) (string, error) {
	path, err := ResolveSSHConfigPath(sshConfigPath)
	if err != nil {
		return "", fmt.Errorf("invalid ssh config path: %w", err)
	}
	sshConfigPath = path

	targetPath := sshConfigPath
	if sshConfigIncludePath != "" {
		includePath, err := ResolveSSHConfigPath(sshConfigIncludePath)
		if err != nil {
			return "", fmt.Errorf("invalid ssh config include path: %w", err)
		}
		targetPath = includePath
	}

	user := "root"
	_, err = transformHostSection(targetPath, workspaceID+"."+"devpod", func(line string) string {
		splitted := strings.Split(strings.ToLower(strings.TrimSpace(line)), " ")
		if len(splitted) == 2 && splitted[0] == "user" {
			user = strings.Trim(splitted[1], "\"")
		}

		return line
	})
	if err != nil {
		return "", err
	}

	return user, nil
}

func RemoveFromConfig(workspaceID string, sshConfigPath string, sshConfigIncludePath string, log log.Logger) error {
	configLock.Lock()
	defer configLock.Unlock()

	targetPath := sshConfigPath
	if sshConfigIncludePath != "" {
		targetPath = sshConfigIncludePath
	}

	newFile, err := removeFromConfig(targetPath, workspaceID+"."+"devpod")
	if err != nil {
		return fmt.Errorf("parse ssh config: %w", err)
	}

	return writeSSHConfig(targetPath, newFile, log)
}

func writeSSHConfig(path, content string, log log.Logger) error {
	//nolint:gosec // SSH config directory needs standard permissions
	err := os.MkdirAll(filepath.Dir(path), 0o755)
	if err != nil {
		log.Debugf("error creating ssh directory: %v", err)
	}

	err = os.WriteFile(path, []byte(content), 0o600)
	if err != nil {
		return fmt.Errorf("write ssh config: %w", err)
	}

	return nil
}

func ResolveSSHConfigPath(sshConfigPath string) (string, error) {
	homeDir, err := util.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}

	if sshConfigPath == "" {
		return filepath.Join(homeDir, ".ssh", "config"), nil
	}

	if strings.HasPrefix(sshConfigPath, "~/") {
		sshConfigPath = strings.Replace(sshConfigPath, "~", homeDir, 1)
	}

	return filepath.Abs(sshConfigPath)
}

func removeFromConfig(path, host string) (string, error) {
	return transformHostSection(path, host, func(line string) string {
		return ""
	})
}

func transformHostSection(path, host string, transform func(line string) string) (string, error) {
	var reader io.Reader
	f, err := os.Open(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", err
		}

		reader = strings.NewReader("")
	} else {
		reader = f
		defer func() { _ = f.Close() }()
	}

	configScanner := scanner.NewScanner(reader)
	newLines := []string{}
	inSection := false
	startMarker := MarkerStartPrefix + host
	endMarker := MarkerEndPrefix + host
	for configScanner.Scan() {
		text := configScanner.Text()
		if strings.HasPrefix(text, startMarker) {
			inSection = true
		} else if strings.HasPrefix(text, endMarker) {
			inSection = false
		} else if !inSection {
			newLines = append(newLines, text)
		} else if inSection {
			text = transform(text)
			if text != "" {
				newLines = append(newLines, text)
			}
		}
	}
	if configScanner.Err() != nil {
		return "", fmt.Errorf("parse ssh config: %w", err)
	}

	// remove residual empty line at start file
	if len(newLines) > 0 && newLines[0] == "" {
		newLines = newLines[1:]
	}

	return strings.Join(newLines, "\n"), nil
}
