package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/pkg/command"
	"github.com/skevetter/devpod/pkg/compress"
	provider2 "github.com/skevetter/devpod/pkg/provider"
	"github.com/skevetter/devpod/pkg/version"
	"github.com/skevetter/log"
)

const DefaultInactivityTimeout = time.Minute * 20

const ContainerDevPodHelperLocation = "/usr/local/bin/devpod"

const RemoteDevPodHelperLocation = "/tmp/devpod"

const ContainerActivityFile = "/tmp/devpod.activity"

const defaultAgentDownloadURL = "https://github.com/skevetter/devpod/releases/download/"

const EnvDevPodAgentURL = "DEVPOD_AGENT_URL"

const EnvDevPodAgentPreferDownload = "DEVPOD_AGENT_PREFER_DOWNLOAD"

const WorkspaceBusyFile = "workspace.lock"

func DefaultAgentDownloadURL() string {
	devPodAgentURL := os.Getenv(EnvDevPodAgentURL)
	if devPodAgentURL != "" {
		return strings.TrimRight(devPodAgentURL, "/")
	}

	if version.GetVersion() == version.DevVersion {
		return "https://github.com/skevetter/devpod/releases/latest/download"
	}

	return defaultAgentDownloadURL + version.GetVersion()
}

func DecodeContainerWorkspaceInfo(workspaceInfoRaw string) (*provider2.ContainerWorkspaceInfo, string, error) {
	decoded, err := compress.Decompress(workspaceInfoRaw)
	if err != nil {
		return nil, "", fmt.Errorf("decode workspace info %w", err)
	}

	workspaceInfo := &provider2.ContainerWorkspaceInfo{}
	err = json.Unmarshal([]byte(decoded), workspaceInfo)
	if err != nil {
		return nil, "", fmt.Errorf("parse workspace info %w", err)
	}

	return workspaceInfo, decoded, nil
}

func DecodeWorkspaceInfo(workspaceInfoRaw string) (*provider2.AgentWorkspaceInfo, string, error) {
	decoded, err := compress.Decompress(workspaceInfoRaw)
	if err != nil {
		return nil, "", fmt.Errorf("decode workspace info %w", err)
	}

	workspaceInfo := &provider2.AgentWorkspaceInfo{}
	err = json.Unmarshal([]byte(decoded), workspaceInfo)
	if err != nil {
		return nil, "", fmt.Errorf("parse workspace info %w", err)
	}

	return workspaceInfo, decoded, nil
}

func readAgentWorkspaceInfo(agentFolder, context, id string) (*provider2.AgentWorkspaceInfo, error) {
	// get workspace folder
	workspaceDir, err := GetAgentWorkspaceDir(agentFolder, context, id)
	if err != nil {
		return nil, err
	}

	// parse agent workspace info
	return ParseAgentWorkspaceInfo(filepath.Join(workspaceDir, provider2.WorkspaceConfigFile))
}

func ParseAgentWorkspaceInfo(workspaceConfigFile string) (*provider2.AgentWorkspaceInfo, error) {
	// read workspace config
	out, err := os.ReadFile(workspaceConfigFile)
	if err != nil {
		return nil, err
	}

	// json unmarshal
	workspaceInfo := &provider2.AgentWorkspaceInfo{}
	err = json.Unmarshal(out, workspaceInfo)
	if err != nil {
		return nil, fmt.Errorf("parse workspace info %w", err)
	}

	workspaceInfo.Origin = filepath.Dir(workspaceConfigFile)
	return workspaceInfo, nil
}

func ReadAgentWorkspaceInfo(agentFolder, context, id string, log log.Logger) (bool, *provider2.AgentWorkspaceInfo, error) {
	log.WithFields(logrus.Fields{
		"agentFolder": agentFolder,
		"context":     context,
		"workspaceId": id,
	}).Debug("starting to read agent workspace info")

	workspaceInfo, err := readAgentWorkspaceInfo(agentFolder, context, id)
	if err != nil && !errors.Is(err, ErrFindAgentHomeFolder) && !errors.Is(err, os.ErrPermission) {
		log.WithFields(logrus.Fields{
			"error":       err,
			"agentFolder": agentFolder,
			"context":     context,
			"workspaceId": id,
		}).Error("failed to read agent workspace info")
		return false, nil, err
	}

	if errors.Is(err, ErrFindAgentHomeFolder) {
		log.WithFields(logrus.Fields{
			"agentFolder": agentFolder,
			"context":     context,
			"workspaceId": id,
		}).Debug("agent home folder not found")
	}

	if errors.Is(err, os.ErrPermission) {
		log.WithFields(logrus.Fields{
			"agentFolder": agentFolder,
			"context":     context,
			"workspaceId": id,
		}).Debug("permission denied reading workspace info")
	}

	// check if we need to become root
	log.Debug("checking if root privileges are required")
	shouldExit, err := rerunAsRoot(workspaceInfo, log)
	if err != nil {
		log.WithFields(logrus.Fields{
			"error": err,
		}).Error("failed to rerun as root")
		return false, nil, fmt.Errorf("rerun as root %w", err)
	} else if shouldExit {
		log.Debug("rerunning as root, exiting current process")
		return true, nil, nil
	} else if workspaceInfo == nil {
		log.Debug("no workspace info available and not rerunning as root")
		return false, nil, ErrFindAgentHomeFolder
	}

	log.WithFields(logrus.Fields{
		"workspaceId": workspaceInfo.Workspace.ID,
		"driver":      workspaceInfo.Agent.Driver,
	}).Debug("read agent workspace info")
	return false, workspaceInfo, nil
}

func WorkspaceInfo(workspaceInfoEncoded string, log log.Logger) (bool, *provider2.AgentWorkspaceInfo, error) {
	return decodeWorkspaceInfoAndWrite(workspaceInfoEncoded, false, nil, log)
}

func WriteWorkspaceInfo(workspaceInfoEncoded string, log log.Logger) (bool, *provider2.AgentWorkspaceInfo, error) {
	return WriteWorkspaceInfoAndDeleteOld(workspaceInfoEncoded, nil, log)
}

func WriteWorkspaceInfoAndDeleteOld(workspaceInfoEncoded string, deleteWorkspace func(workspaceInfo *provider2.AgentWorkspaceInfo, log log.Logger) error, log log.Logger) (bool, *provider2.AgentWorkspaceInfo, error) {
	return decodeWorkspaceInfoAndWrite(workspaceInfoEncoded, true, deleteWorkspace, log)
}

func decodeWorkspaceInfoAndWrite(
	workspaceInfoEncoded string,
	writeInfo bool,
	deleteWorkspace func(workspaceInfo *provider2.AgentWorkspaceInfo, log log.Logger) error,
	log log.Logger,
) (bool, *provider2.AgentWorkspaceInfo, error) {
	log.WithFields(logrus.Fields{
		"workspaceEncodedLength": len(workspaceInfoEncoded),
		"writeInfo":              writeInfo,
	}).Debug("starting to decode and write workspace info")

	workspaceInfo, _, err := DecodeWorkspaceInfo(workspaceInfoEncoded)
	if err != nil {
		log.WithFields(logrus.Fields{
			"error":                  err,
			"workspaceEncodedLength": len(workspaceInfoEncoded),
		}).Error("failed to decode workspace info")
		return false, nil, err
	}

	log.WithFields(logrus.Fields{
		"workspaceId": workspaceInfo.Workspace.ID,
		"context":     workspaceInfo.Workspace.Context,
		"driver":      workspaceInfo.Agent.Driver,
	}).Debug("decoded workspace info")

	// check if we need to become root
	log.Debug("checking if root privileges are required")
	shouldExit, err := rerunAsRoot(workspaceInfo, log)
	if err != nil {
		log.WithFields(logrus.Fields{
			"error": err,
		}).Error("failed to rerun as root")
		return false, nil, fmt.Errorf("rerun as root %w", err)
	} else if shouldExit {
		log.Debug("rerunning as root, exiting current process")
		return true, nil, nil
	}

	// write to workspace folder
	log.WithFields(logrus.Fields{
		"dataPath":    workspaceInfo.Agent.DataPath,
		"context":     workspaceInfo.Workspace.Context,
		"workspaceId": workspaceInfo.Workspace.ID,
	}).Debug("creating agent workspace directory")
	workspaceDir, err := CreateAgentWorkspaceDir(workspaceInfo.Agent.DataPath, workspaceInfo.Workspace.Context, workspaceInfo.Workspace.ID)
	if err != nil {
		log.WithFields(logrus.Fields{
			"error":       err,
			"dataPath":    workspaceInfo.Agent.DataPath,
			"context":     workspaceInfo.Workspace.Context,
			"workspaceId": workspaceInfo.Workspace.ID,
		}).Error("failed to create agent workspace directory")
		return false, nil, err
	}

	log.WithFields(logrus.Fields{
		"workspaceDir": workspaceDir,
	}).Debug("using workspace dir")

	// check if workspace config already exists
	workspaceConfig := filepath.Join(workspaceDir, provider2.WorkspaceConfigFile)
	if deleteWorkspace != nil {
		log.WithFields(logrus.Fields{
			"configFile": workspaceConfig,
		}).Debug("checking for existing workspace config")

		oldWorkspaceInfo, _ := ParseAgentWorkspaceInfo(workspaceConfig)
		if oldWorkspaceInfo != nil && oldWorkspaceInfo.Workspace.UID != workspaceInfo.Workspace.UID {
			// delete the old workspace
			log.WithFields(logrus.Fields{
				"workspaceId": oldWorkspaceInfo.Workspace.ID,
				"oldUid":      oldWorkspaceInfo.Workspace.UID,
				"newUid":      workspaceInfo.Workspace.UID,
			}).Info("delete old workspace")

			err = deleteWorkspace(oldWorkspaceInfo, log)
			if err != nil {
				log.WithFields(logrus.Fields{
					"error":       err,
					"workspaceId": oldWorkspaceInfo.Workspace.ID,
				}).Error("failed to delete old workspace")
				return false, nil, fmt.Errorf("delete old workspace %w", err)
			}

			// recreate workspace folder again
			log.Debug("recreating workspace directory after deletion")
			workspaceDir, err = CreateAgentWorkspaceDir(workspaceInfo.Agent.DataPath, workspaceInfo.Workspace.Context, workspaceInfo.Workspace.ID)
			if err != nil {
				log.WithFields(logrus.Fields{
					"error":    err,
					"dataPath": workspaceInfo.Agent.DataPath,
				}).Error("failed to recreate workspace directory")
				return false, nil, err
			}
		}
	}

	// check content folder for local folder workspace source
	//
	// We don't want to initialize the content folder with the value of the local workspace folder
	// if we're running in proxy mode.
	// We only have write access to /var/lib/loft/* by default causing nearly all local folders to run into permissions issues
	if workspaceInfo.Workspace.Source.LocalFolder != "" && !workspaceInfo.CLIOptions.Platform.Enabled {
		log.WithFields(logrus.Fields{
			"localFolder":     workspaceInfo.Workspace.Source.LocalFolder,
			"workspaceOrigin": workspaceInfo.WorkspaceOrigin,
		}).Debug("checking local folder workspace source")

		_, err = os.Stat(workspaceInfo.WorkspaceOrigin)
		if err == nil {
			workspaceInfo.ContentFolder = workspaceInfo.Workspace.Source.LocalFolder
			log.WithFields(logrus.Fields{
				"contentFolder": workspaceInfo.ContentFolder,
			}).Debug("set content folder to local folder")
		} else {
			log.WithFields(logrus.Fields{
				"error":           err,
				"workspaceOrigin": workspaceInfo.WorkspaceOrigin,
			}).Debug("workspace origin not accessible")
		}
	}

	// set content folder
	if workspaceInfo.ContentFolder == "" {
		workspaceInfo.ContentFolder = GetAgentWorkspaceContentDir(workspaceDir)
		log.WithFields(logrus.Fields{
			"contentFolder": workspaceInfo.ContentFolder,
		}).Debug("set content folder to default location")
	}

	// write workspace info
	if writeInfo {
		log.WithFields(logrus.Fields{
			"configFile": workspaceConfig,
		}).Debug("writing workspace info to file")

		err = writeWorkspaceInfo(workspaceConfig, workspaceInfo)
		if err != nil {
			log.WithFields(logrus.Fields{
				"error":      err,
				"configFile": workspaceConfig,
			}).Error("failed to write workspace info")
			return false, nil, err
		}

		log.WithFields(logrus.Fields{
			"configFile": workspaceConfig,
		}).Debug("wrote workspace info")
	}

	workspaceInfo.Origin = workspaceDir
	log.WithFields(logrus.Fields{
		"workspaceId":   workspaceInfo.Workspace.ID,
		"origin":        workspaceInfo.Origin,
		"contentFolder": workspaceInfo.ContentFolder,
	}).Debug("processed workspace info")

	return false, workspaceInfo, nil
}

func CreateWorkspaceBusyFile(folder string) {
	filePath := filepath.Join(folder, WorkspaceBusyFile)
	_, err := os.Stat(filePath)
	if err == nil {
		return
	}

	_ = os.WriteFile(filePath, nil, 0600)
}

func HasWorkspaceBusyFile(folder string) bool {
	filePath := filepath.Join(folder, WorkspaceBusyFile)
	_, err := os.Stat(filePath)
	return err == nil
}

func DeleteWorkspaceBusyFile(folder string) {
	_ = os.Remove(filepath.Join(folder, WorkspaceBusyFile))
}

func writeWorkspaceInfo(file string, workspaceInfo *provider2.AgentWorkspaceInfo) error {
	// copy workspace info
	cloned := provider2.CloneAgentWorkspaceInfo(workspaceInfo)

	// never save cli options
	cloned.CLIOptions = provider2.CLIOptions{}

	// encode workspace info
	encoded, err := json.Marshal(workspaceInfo)
	if err != nil {
		return err
	}

	// write workspace config
	err = os.WriteFile(file, encoded, 0600)
	if err != nil {
		return fmt.Errorf("write workspace config file %w", err)
	}

	return nil
}

func rerunAsRoot(workspaceInfo *provider2.AgentWorkspaceInfo, log log.Logger) (bool, error) {
	// check if root is required
	if runtime.GOOS != "linux" || os.Getuid() == 0 || (workspaceInfo != nil && workspaceInfo.Agent.Local == "true") {
		return false, nil
	}

	// check if we can reach docker with no problems
	dockerRootRequired := false
	if workspaceInfo != nil && (workspaceInfo.Agent.Driver == "" || workspaceInfo.Agent.Driver == provider2.DockerDriver) {
		var err error
		dockerRootRequired, err = dockerReachable(workspaceInfo.Agent.Docker.Path, workspaceInfo.Agent.Docker.Env)
		if err != nil {
			log.WithFields(logrus.Fields{
				"error": err,
			}).Debug("error trying to reach docker daemon")
			dockerRootRequired = true
		}
	}

	// check if daemon needs to be installed
	agentRootRequired := workspaceInfo == nil || len(workspaceInfo.Agent.Exec.Shutdown) > 0

	// check if root required
	if !dockerRootRequired && !agentRootRequired {
		log.Debug("no root required, because neither docker nor agent daemon needs to be installed")
		return false, nil
	}

	// execute ourself as root
	binary, err := os.Executable()
	if err != nil {
		return false, err
	}

	// call ourself
	args := []string{"--preserve-env", binary}
	args = append(args, os.Args[1:]...)
	log.WithFields(logrus.Fields{
		"command": strings.Join(args, " "),
	}).Debug("re-run as root")
	cmd := exec.Command("sudo", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		return false, err
	}

	return true, nil
}

type Exec func(ctx context.Context, user string, command string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error

func Tunnel(
	ctx context.Context,
	exec Exec,
	user string,
	stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer,
	log log.Logger,
	timeout time.Duration,
) error {
	err := InjectAgent(&InjectOptions{
		Ctx: ctx,
		Exec: func(ctx context.Context, command string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
			return exec(ctx, "root", command, stdin, stdout, stderr)
		},
		IsLocal:                     false,
		RemoteAgentPath:             ContainerDevPodHelperLocation,
		DownloadURL:                 DefaultAgentDownloadURL(),
		PreferDownloadFromRemoteUrl: Bool(false),
		Log:                         log,
		Timeout:                     timeout,
	})
	if err != nil {
		return err
	}

	// build command
	command := fmt.Sprintf("'%s' helper ssh-server --stdio", ContainerDevPodHelperLocation)
	if log.GetLevel() == logrus.DebugLevel {
		command += " --debug"
	}
	if user == "" {
		user = "root"
	}

	// create tunnel
	err = exec(ctx, user, command, stdin, stdout, stderr)
	if err != nil {
		return err
	}

	return nil
}

func dockerReachable(dockerOverride string, envs map[string]string) (bool, error) {
	docker := "docker"
	if dockerOverride != "" {
		docker = dockerOverride
	}

	if !command.Exists(docker) {
		// if docker is overridden, we assume that there is an error as we don't know how to install the command provided
		if dockerOverride != "" {
			return false, fmt.Errorf("docker command '%s' not found", dockerOverride)
		}
		// we need root to install docker
		return true, nil
	}

	cmd := exec.Command(docker, "ps")
	if len(envs) > 0 {
		newEnvs := os.Environ()
		for k, v := range envs {
			newEnvs = append(newEnvs, k+"="+v)
		}
		cmd.Env = newEnvs
	}

	_, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(err.Error(), "permission denied") {
			if dockerOverride == "" {
				return true, nil
			}
		}

		return false, fmt.Errorf("%s ps %w", docker, err)
	}

	return false, nil
}
