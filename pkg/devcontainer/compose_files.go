package devcontainer

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	"github.com/skevetter/devpod/pkg/devcontainer/config"
)

func (r *runner) dockerComposeProjectFiles(parsedConfig *config.SubstitutedConfig) ([]string, []string, []string, error) {
	envFiles, err := r.getEnvFiles()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("get env files: %w", err)
	}

	composeFiles, err := r.getDockerComposeFilePaths(parsedConfig, envFiles)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("get docker compose file paths: %w", err)
	}

	var args []string
	for _, configFile := range composeFiles {
		args = append(args, "-f", configFile)
	}

	for _, envFile := range envFiles {
		args = append(args, "--env-file", envFile)
	}

	return composeFiles, envFiles, args, nil
}

func (r *runner) getDockerComposeFilePaths(parsedConfig *config.SubstitutedConfig, envFiles []string) ([]string, error) {
	configFileDir := filepath.Dir(parsedConfig.Config.Origin)

	// Use docker compose files from config
	var composeFiles []string
	if len(parsedConfig.Config.DockerComposeFile) > 0 {
		for _, composeFile := range parsedConfig.Config.DockerComposeFile {
			absPath := composeFile
			if !filepath.IsAbs(composeFile) {
				absPath = filepath.Join(configFileDir, composeFile)
			}
			composeFiles = append(composeFiles, absPath)
		}

		return composeFiles, nil
	}

	// Use docker compose files from $COMPOSE_FILE environment variable
	envComposeFile := os.Getenv("COMPOSE_FILE")

	// Load docker compose files from $COMPOSE_FILE in .env file
	if envComposeFile == "" {
		for _, envFile := range envFiles {
			env, err := godotenv.Read(envFile)
			if err != nil {
				return nil, err
			}

			if env["COMPOSE_FILE"] != "" {
				envComposeFile = env["COMPOSE_FILE"]
				break
			}
		}
	}

	if envComposeFile != "" {
		return filepath.SplitList(envComposeFile), nil
	}

	return nil, nil
}

func (r *runner) getEnvFiles() ([]string, error) {
	var envFiles []string
	envFile := path.Join(r.LocalWorkspaceFolder, ".env")
	envFileStat, err := os.Stat(envFile)
	if err == nil && envFileStat.Mode().IsRegular() {
		envFiles = append(envFiles, envFile)
	}
	return envFiles, nil
}

func checkForPersistedFile(files []string, prefix string) (foundLabel bool, fileExists bool, filePath string, err error) {
	for _, file := range files {
		if !strings.HasPrefix(file, prefix) {
			continue
		}

		stat, err := os.Stat(file)
		if err == nil && stat.Mode().IsRegular() {
			return true, true, file, nil
		} else if os.IsNotExist(err) {
			return true, false, file, nil
		}
	}

	return false, false, "", nil
}

func getDockerComposeFolder(workspaceOriginFolder string) string {
	return filepath.Join(workspaceOriginFolder, ".docker-compose")
}
