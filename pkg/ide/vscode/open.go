package vscode

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/skevetter/devpod/pkg/command"
	"github.com/skevetter/log"
	"github.com/skratchdot/open-golang/open"
)

const containersExtension = "ms-vscode-remote.remote-containers"

type OpenParams struct {
	Workspace string
	Folder    string
	NewWindow bool
	Flavor    Flavor
	Log       log.Logger
}

type openConfig struct {
	protocol     string
	cliName      string
	macAppPath   string
	sshExtension string
}

var openConfigs = map[Flavor]openConfig{
	FlavorStable: {
		protocol:     "vscode://",
		cliName:      "code",
		macAppPath:   "/Applications/Visual Studio Code.app/Contents/Resources/app/bin/code",
		sshExtension: "ms-vscode-remote.remote-ssh",
	},
	FlavorInsiders: {
		protocol:     "vscode-insiders://",
		cliName:      "code-insiders",
		macAppPath:   "/Applications/Visual Studio Code - Insiders.app/Contents/Resources/app/bin/code",
		sshExtension: "ms-vscode-remote.remote-ssh",
	},
	FlavorCursor: {
		protocol:     "cursor://",
		cliName:      "cursor",
		macAppPath:   "/Applications/Cursor.app/Contents/Resources/app/bin/cursor",
		sshExtension: "ms-vscode-remote.remote-ssh",
	},
	FlavorPositron: {
		protocol:     "positron://",
		cliName:      "positron",
		macAppPath:   "/Applications/Positron.app/Contents/Resources/app/bin/positron",
		sshExtension: "ms-vscode-remote.remote-ssh",
	},
	FlavorCodium: {
		protocol:     "codium://",
		cliName:      "codium",
		macAppPath:   "/Applications/Codium.app/Contents/Resources/app/bin/codium",
		sshExtension: "jeanp413.open-remote-ssh",
	},
	FlavorWindsurf: {
		protocol:     "windsurf://",
		cliName:      "windsurf",
		macAppPath:   "/Applications/Windsurf.app/Contents/Resources/app/bin/windsurf",
		sshExtension: "ms-vscode-remote.remote-ssh",
	},
	FlavorAntigravity: {
		protocol:     "antigravity://",
		cliName:      "agy",
		macAppPath:   "/Applications/Antigravity.app/Contents/Resources/app/bin/agy",
		sshExtension: "ms-vscode-remote.remote-ssh",
	},
}

func Open(ctx context.Context, params OpenParams) error {
	cliErr := openViaCLI(ctx, params)
	if cliErr == nil {
		params.Log.Infof("opened %s via CLI", params.Flavor.DisplayName())
		return nil
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	browserErr := openViaBrowser(params)
	if browserErr == nil {
		params.Log.Infof("opened %s via browser", params.Flavor.DisplayName())
		return nil
	}

	return errors.Join(cliErr, browserErr)
}

func openViaBrowser(params OpenParams) error {
	config, ok := openConfigs[params.Flavor]
	if !ok {
		return fmt.Errorf("unknown flavor %s", params.Flavor)
	}

	openURL := fmt.Sprintf("%svscode-remote/ssh-remote+%s.devpod/%s", config.protocol, params.Workspace, params.Folder)
	if params.NewWindow {
		openURL += "?windowId=_blank"
	}

	err := open.Run(openURL)
	if err != nil {
		params.Log.Errorf("flavor %s is not installed on host device: %v", params.Flavor.DisplayName(), err)
		return err
	}

	return nil
}

func openViaCLI(ctx context.Context, params OpenParams) error {
	config, ok := openConfigs[params.Flavor]
	if !ok {
		return fmt.Errorf("unknown flavor %s", params.Flavor)
	}

	cliPath := getCLIPath(config)
	if cliPath == "" {
		return fmt.Errorf("flavor %s binary is not found", params.Flavor)
	}

	hasSSHExtension, hasContainersExtension, err := listInstalledExtensions(ctx, cliPath, config.sshExtension)
	if err != nil {
		return err
	}

	if !hasSSHExtension {
		if err := ensureSSHExtension(ctx, cliPath, config.sshExtension, params.Log); err != nil {
			return err
		}
	}

	args := buildOpenArgs(params.Workspace, params.Folder, params.NewWindow, hasContainersExtension)
	params.Log.Debugf("flavor %s command %s %s", params.Flavor.DisplayName(), cliPath, strings.Join(args, " "))
	out, err := exec.CommandContext(ctx, cliPath, args...).CombinedOutput()
	if err != nil {
		return command.WrapCommandError(out, err)
	}

	return nil
}

func listInstalledExtensions(ctx context.Context, cliPath, sshExtension string) (hasSSH, hasContainers bool, err error) {
	out, err := exec.CommandContext(ctx, cliPath, "--list-extensions").CombinedOutput()
	if err != nil {
		return false, false, command.WrapCommandError(out, err)
	}

	for ext := range strings.SplitSeq(string(out), "\n") {
		ext = strings.TrimSpace(ext)
		switch ext {
		case sshExtension:
			hasSSH = true
		case containersExtension:
			hasContainers = true
		}

		if hasSSH && hasContainers {
			break
		}
	}

	return hasSSH, hasContainers, nil
}

func ensureSSHExtension(ctx context.Context, cliPath, sshExtension string, log log.Logger) error {
	args := []string{"--install-extension", sshExtension}
	log.Debugf("%s %s", cliPath, strings.Join(args, " "))
	out, err := exec.CommandContext(ctx, cliPath, args...).CombinedOutput()
	if err != nil {
		return command.WrapCommandError(out, err)
	}
	return nil
}

func buildOpenArgs(workspace, folder string, newWindow, hasContainersExtension bool) []string {
	args := make([]string, 0, 4)

	if hasContainersExtension {
		args = append(args, "--disable-extension", containersExtension)
	}

	if newWindow {
		args = append(args, "--new-window")
	} else {
		args = append(args, "--reuse-window")
	}

	args = append(args, fmt.Sprintf("--folder-uri=vscode-remote://ssh-remote+%s.devpod/%s", workspace, strings.TrimPrefix(folder, "/")))

	return args
}

func getCLIPath(config openConfig) string {
	if command.Exists(config.cliName) {
		return config.cliName
	}

	if runtime.GOOS == "darwin" && command.Exists(config.macAppPath) {
		return config.macAppPath
	}

	return ""
}
