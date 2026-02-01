package dockerinstall

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

func Install(stdout, stderr io.Writer) (string, error) {
	opts := &InstallOptions{
		channel:     getEnv("CHANNEL", ChannelStable),
		version:     getEnv("VERSION", ""),
		downloadURL: getEnv("DOWNLOAD_URL", DefaultDownloadURL),
		repoFile:    getEnv("REPO_FILE", DefaultRepoFile),
		dryRun:      getEnv("DRY_RUN", "") != "",
		stdout:      stdout,
		stderr:      stderr,
	}

	detector := NewDetector()
	validator := NewValidator(opts)

	if err := validator.ValidateOS(detector.DetectOS()); err != nil {
		return "", err
	}

	validator.CheckExistingDocker()
	validator.CheckWSL(detector.IsWSL())

	distro := detector.DetectDistro()
	distro = detector.CheckForked(distro)

	if err := validator.ValidateDistro(distro); err != nil {
		return "", err
	}

	validator.CheckDeprecation(distro)

	shC := getShellCommand(opts)
	if shC == "" {
		return "", fmt.Errorf("no shell command available: sudo or su required")
	}

	installer := createInstaller(distro, opts)
	if installer == nil {
		return "", fmt.Errorf("unsupported distribution: %s", distro.ID)
	}
	if err := installer.Install(shC); err != nil {
		return "", fmt.Errorf("docker installation failed: %w", err)
	}

	echoDockerAsNonroot(opts)

	return findDockerPath(), nil
}

func createInstaller(distro *Distro, opts *InstallOptions) Installer {
	switch distro.ID {
	case "ubuntu", "debian", "raspbian":
		return NewDebianInstaller(distro, opts)
	default:
		return nil
	}
}

func getShellCommand(opts *InstallOptions) string {
	user := getUser()
	if user == "root" {
		if opts.dryRun {
			return ShellEcho
		}
		return "sh -c"
	}

	if commandExists("sudo") {
		if opts.dryRun {
			return ShellEcho
		}
		return "sudo -E sh -c"
	}

	if commandExists("su") {
		if opts.dryRun {
			return ShellEcho
		}
		// Note: su -c does not preserve environment like sudo -E
		// Environment variables may need to be passed explicitly in commands
		return "su -c"
	}

	fprintln(opts.stderr, `Error: installer needs the ability to run commands as root.
commands "sudo" and "su" are not found.`)
	return ""
}

func getUser() string {
	if user := os.Getenv("USER"); user != "" {
		return user
	}
	cmd := exec.Command("id", "-un")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func findDockerPath() string {
	paths := []string{DockerBinPath1, DockerBinPath2, DockerBinPath3}
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return "docker"
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func echoDockerAsNonroot(opts *InstallOptions) {
	if opts.dryRun {
		return
	}

	if commandExists("docker") {
		if _, err := os.Stat(DockerSocket); err == nil {
			fprintf(opts.stderr, "+ docker version\n")
			cmd := exec.Command("docker", "version")
			cmd.Stdout = opts.stdout
			cmd.Stderr = opts.stdout
			_ = cmd.Run()
		}
	}

	fprintln(opts.stdout, `
================================================================================`)
	if versionGte(opts.version, "20.10") {
		fprintln(opts.stdout, `
To run Docker as a non-privileged user, consider setting up the
Docker daemon in rootless mode for your user:

    dockerd-rootless-setuptool.sh install

Visit https://docs.docker.com/go/rootless/ to learn about rootless mode.`)
	}
	fprintln(opts.stdout, `
To run the Docker daemon as a fully privileged service, but granting non-root
users access, refer to https://docs.docker.com/go/daemon-access/

WARNING: Access to the remote API on a privileged Docker daemon is equivalent
         to root access on the host. Refer to the 'Docker daemon attack surface'
         documentation for details: https://docs.docker.com/go/attack-surface/

================================================================================`)
}
