package setup

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/loft-sh/api/v4/pkg/devpod"
	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/pkg/agent/tunnel"
	"github.com/skevetter/devpod/pkg/command"
	copy2 "github.com/skevetter/devpod/pkg/copy"
	"github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/skevetter/devpod/pkg/envfile"
	"github.com/skevetter/devpod/pkg/gitcredentials"
	"github.com/skevetter/log"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const (
	ResultLocation = "/var/run/devpod/result.json"
)

type ContainerSetupConfig struct {
	SetupInfo         *config.Result
	ExtraWorkspaceEnv []string
	ChownProjects     bool
	PlatformOptions   *devpod.PlatformOptions
	TunnelClient      tunnel.TunnelClient
	Log               log.Logger
}

func SetupContainer(ctx context.Context, cfg *ContainerSetupConfig) error {
	rawBytes, err := json.Marshal(cfg.SetupInfo)
	if err != nil {
		cfg.Log.Warnf("error marshal result: %v", err)
	}

	existing, _ := os.ReadFile(ResultLocation)
	if string(rawBytes) != string(existing) {
		err = os.MkdirAll(filepath.Dir(ResultLocation), 0755)
		if err != nil {
			cfg.Log.Warnf("error create %s: %v", filepath.Dir(ResultLocation), err)
		}

		err = os.WriteFile(ResultLocation, rawBytes, 0600)
		if err != nil {
			cfg.Log.Warnf("error write result to %s: %v", ResultLocation, err)
		}
	}

	if err := setupWorkspaceOwnership(cfg); err != nil {
		return err
	}

	if err := setupEnvironment(cfg); err != nil {
		return err
	}

	setupOptionalFeatures(ctx, cfg)

	cfg.Log.Debugf("running lifecycle hooks")
	if err := RunLifecycleHooks(ctx, cfg.SetupInfo, cfg.Log); err != nil {
		return fmt.Errorf("lifecycle hooks: %w", err)
	}

	cfg.Log.Debugf("devcontainer setup completed")
	return nil
}

func setupWorkspaceOwnership(cfg *ContainerSetupConfig) error {
	if err := chownWorkspace(cfg.SetupInfo, cfg.ChownProjects, cfg.Log); err != nil {
		return fmt.Errorf("failed to chown workspace: %w", err)
	}

	if err := linkRootHome(cfg.SetupInfo); err != nil {
		cfg.Log.Errorf("Error linking /home/root: %v", err)
	}

	if err := chownAgentSock(cfg.SetupInfo); err != nil {
		return fmt.Errorf("chown ssh agent sock file: %w", err)
	}

	return nil
}

func setupEnvironment(cfg *ContainerSetupConfig) error {
	cfg.Log.Debugf("patching etc environment")

	if err := patchEtcEnvironment(cfg.SetupInfo.MergedConfig, cfg.Log); err != nil {
		return fmt.Errorf("patch etc environment: %w", err)
	}

	if err := patchEtcEnvironmentFlags(cfg.ExtraWorkspaceEnv, cfg.Log); err != nil {
		return fmt.Errorf("patch etc environment from flags: %w", err)
	}

	if err := patchEtcProfile(); err != nil {
		return fmt.Errorf("patch etc profile: %w", err)
	}

	return nil
}

func setupOptionalFeatures(ctx context.Context, cfg *ContainerSetupConfig) {
	if err := setupKubeConfig(ctx, cfg.SetupInfo, cfg.TunnelClient, cfg.Log); err != nil {
		cfg.Log.Errorf("setup KubeConfig: %v", err)
	}

	if err := setupPlatformGitCredentials(config.GetRemoteUser(cfg.SetupInfo), cfg.PlatformOptions, cfg.Log); err != nil {
		cfg.Log.Errorf("setup platform git credentials: %v", err)
	}
}

func linkRootHome(setupInfo *config.Result) error {
	user := config.GetRemoteUser(setupInfo)
	if user != "root" {
		return nil
	}

	home, err := command.GetHome(user)
	if err != nil {
		return fmt.Errorf("find root home: %w", err)
	} else if home == "/home/root" {
		return nil
	}

	_, err = os.Stat("/home/root")
	if err == nil {
		return nil
	}

	// link /home/root to the root home
	err = os.MkdirAll("/home", 0777)
	if err != nil {
		return fmt.Errorf("create /home folder: %w", err)
	}

	err = os.Symlink(home, "/home/root")
	if err != nil {
		return fmt.Errorf("create symlink: %w", err)
	}

	return nil
}

func chownWorkspace(setupInfo *config.Result, recursive bool, log log.Logger) error {
	user := config.GetRemoteUser(setupInfo)
	exists, err := markerFileExists("chownWorkspace", "")
	if err != nil {
		return err
	} else if exists {
		return nil
	}

	workspaceRoot := filepath.Dir(setupInfo.SubstitutionContext.ContainerWorkspaceFolder)

	if workspaceRoot != "/" {
		log.WithFields(logrus.Fields{
			"user":          user,
			"workspaceRoot": workspaceRoot,
		}).Info("chown workspace")
		err = copy2.Chown(workspaceRoot, user)
		if err != nil {
			log.Warn(err)
		}
	}

	if recursive {
		log.WithFields(logrus.Fields{
			"user":            user,
			"workspaceFolder": setupInfo.SubstitutionContext.ContainerWorkspaceFolder,
		}).Info("chown workspace recursively")
		err = copy2.ChownR(setupInfo.SubstitutionContext.ContainerWorkspaceFolder, user)
		// do not exit on error, we can have non-fatal errors
		if err != nil {
			log.Warn(err)
		}
	}

	return nil
}

func patchEtcProfile() error {
	exists, err := markerFileExists("patchEtcProfile", "")
	if err != nil {
		return err
	} else if exists {
		return nil
	}

	out, err := exec.Command("sh", "-c", `sed -i -E 's/((^|\s)PATH=)([^\$]*)$/\1${PATH:-\3}/g' /etc/profile || true`).CombinedOutput()
	if err != nil {
		return fmt.Errorf("create remote environment: %v: %w", string(out), err)
	}

	return nil
}

func patchEtcEnvironmentFlags(workspaceEnv []string, log log.Logger) error {
	if len(workspaceEnv) == 0 {
		return nil
	}

	// make sure we sort the strings
	sort.Strings(workspaceEnv)

	// check if we need to update env
	exists, err := markerFileExists("patchEtcEnvironmentFlags", strings.Join(workspaceEnv, "\n"))
	if err != nil {
		return err
	} else if exists {
		return nil
	}

	// update env
	envfile.MergeAndApply(config.ListToObject(workspaceEnv), log)
	return nil
}

func patchEtcEnvironment(mergedConfig *config.MergedDevContainerConfig, log log.Logger) error {
	if len(mergedConfig.RemoteEnv) == 0 {
		return nil
	}

	// build remote env
	remoteEnvs := []string{}
	for k, v := range mergedConfig.RemoteEnv {
		remoteEnvs = append(remoteEnvs, k+"=\""+v+"\"")
	}
	sort.Strings(remoteEnvs)

	// check if we need to update env
	exists, err := markerFileExists("patchEtcEnvironment", strings.Join(remoteEnvs, "\n"))
	if err != nil {
		return err
	} else if exists {
		return nil
	}

	// update env
	envfile.MergeAndApply(mergedConfig.RemoteEnv, log)
	return nil
}

func chownAgentSock(setupInfo *config.Result) error {
	user := config.GetRemoteUser(setupInfo)
	agentSockFile := os.Getenv("SSH_AUTH_SOCK")
	if agentSockFile != "" {
		err := copy2.ChownR(filepath.Dir(agentSockFile), user)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	return nil
}

// setupKubeConfig retrieves and stores a KubeConfig file in the default location `$HOME/.kube/config`.
// It merges our KubeConfig with existing ones.
func setupKubeConfig(ctx context.Context, setupInfo *config.Result, tunnelClient tunnel.TunnelClient, log log.Logger) error {
	exists, err := markerFileExists("setupKubeConfig", "")
	if err != nil {
		return err
	}
	if exists || tunnelClient == nil {
		return nil
	}
	log.Info("setup KubeConfig")

	kubeConfigRes, err := tunnelClient.KubeConfig(ctx, &tunnel.Message{})
	if err != nil {
		return err
	}
	if kubeConfigRes.Message == "" {
		return nil
	}

	user := config.GetRemoteUser(setupInfo)
	homeDir, err := command.GetHome(user)
	if err != nil {
		return err
	}

	kubeDir := filepath.Join(homeDir, ".kube")
	if err := createKubeDir(kubeDir); err != nil {
		return err
	}

	configPath := filepath.Join(kubeDir, "config")
	if err := mergeKubeConfig(configPath, kubeConfigRes.Message); err != nil {
		return err
	}

	return copy2.ChownR(kubeDir, user)
}

func createKubeDir(kubeDir string) error {
	err := os.Mkdir(kubeDir, 0755)
	if err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	return nil
}

func mergeKubeConfig(configPath, newConfigData string) error {
	existingConfig, err := clientcmd.LoadFromFile(configPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if existingConfig == nil {
		existingConfig = clientcmdapi.NewConfig()
	}

	kubeConfig, err := clientcmd.Load([]byte(newConfigData))
	if err != nil {
		return err
	}

	maps.Copy(existingConfig.Clusters, kubeConfig.Clusters)
	maps.Copy(existingConfig.AuthInfos, kubeConfig.AuthInfos)
	maps.Copy(existingConfig.Contexts, kubeConfig.Contexts)
	existingConfig.CurrentContext = kubeConfig.CurrentContext

	return clientcmd.WriteToFile(*existingConfig, configPath)
}

func markerFileExists(markerName string, markerContent string) (bool, error) {
	markerName = filepath.Join("/var/devpod", markerName+".marker")
	t, err := os.ReadFile(markerName)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	} else if err == nil && (markerContent == "" || string(t) == markerContent) {
		return true, nil
	}

	// write marker
	_ = os.MkdirAll(filepath.Dir(markerName), 0777)
	err = os.WriteFile(markerName, []byte(markerContent), 0644)
	if err != nil {
		return false, fmt.Errorf("write marker: %w", err)
	}

	return false, nil
}

func setupPlatformGitCredentials(userName string, platformOptions *devpod.PlatformOptions, log log.Logger) error {
	// platform is not enabled, skip
	if !platformOptions.Enabled {
		return nil
	}

	// setup platform git user
	if platformOptions.UserCredentials.GitUser != "" && platformOptions.UserCredentials.GitEmail != "" {
		gitUser, err := gitcredentials.GetUser(userName)
		if err == nil && gitUser.Name == "" && gitUser.Email == "" {
			log.Info("Setup workspace git user and email")
			err := gitcredentials.SetUser(userName, &gitcredentials.GitUser{
				Name:  platformOptions.UserCredentials.GitUser,
				Email: platformOptions.UserCredentials.GitEmail,
			})
			if err != nil {
				return fmt.Errorf("set git user: %w", err)
			}
		}
	}

	// setup platform git http credentials
	err := setupPlatformGitHTTPCredentials(userName, platformOptions, log)
	if err != nil {
		log.Errorf("Error setting up platform git http credentials: %v", err)
	}

	// setup platform git ssh keys
	err = setupPlatformGitSSHKeys(userName, platformOptions, log)
	if err != nil {
		log.Errorf("Error setting up platform git ssh keys: %v", err)
	}

	return nil
}
func setupPlatformGitHTTPCredentials(userName string, platformOptions *devpod.PlatformOptions, log log.Logger) error {
	if !platformOptions.Enabled || len(platformOptions.UserCredentials.GitHttp) == 0 {
		return nil
	}

	log.Info("Setup platform user git http credentials")
	binaryPath, err := os.Executable()
	if err != nil {
		return err
	}
	err = gitcredentials.ConfigureHelper(binaryPath, userName, -1)
	if err != nil {
		return fmt.Errorf("configure git helper: %w", err)
	}

	return nil
}

func setupPlatformGitSSHKeys(userName string, platformOptions *devpod.PlatformOptions, log log.Logger) error {
	if !platformOptions.Enabled || len(platformOptions.UserCredentials.GitSsh) == 0 {
		return nil
	}

	log.Info("Setup platform user git ssh keys")
	homeFolder, err := command.GetHome(userName)
	if err != nil {
		return err
	}

	// write ssh keys to ~/.ssh/id_rsa
	sshFolder := filepath.Join(homeFolder, ".ssh")
	err = os.MkdirAll(sshFolder, 0700)
	if err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	_ = copy2.Chown(sshFolder, userName)

	// delete previous keys
	files, err := os.ReadDir(sshFolder)
	if err != nil {
		return err
	}
	for _, file := range files {
		if !strings.HasPrefix(file.Name(), "platform_git_ssh_") {
			continue
		}

		fileName := strings.TrimPrefix(file.Name(), "platform_git_ssh_")
		index, err := strconv.Atoi(fileName)
		if err != nil {
			continue
		}
		if index >= len(platformOptions.UserCredentials.GitSsh) {
			continue
		}

		err = os.Remove(filepath.Join(sshFolder, file.Name()))
		if err != nil {
			log.Warnf("Error removing previous platform git ssh key: %v", err)
		}
	}

	// write new keys
	for i, key := range platformOptions.UserCredentials.GitSsh {
		fileName := filepath.Join(sshFolder, fmt.Sprintf("platform_git_ssh_%d", i))

		// base64 decode before writing to file
		decoded, err := base64.StdEncoding.DecodeString(key.Key)
		if err != nil {
			log.Warnf("Error decoding platform git ssh key: %v", err)
			continue
		}
		err = os.WriteFile(fileName, decoded, 0600)
		if err != nil {
			log.Warnf("Error writing platform git ssh key: %v", err)
			continue
		}

		err = copy2.Chown(fileName, userName)
		// do not exit on error, we can have non-fatal errors
		if err != nil {
			log.Warnf("Error chowning platform git ssh keys: %v", err)
		}
	}

	return nil
}
