package dockerinstall

import "time"

const (
	// Channels.
	ChannelStable = "stable"

	// Timeouts.
	DefaultTimeout      = 5 * time.Minute
	ExistingDockerDelay = 20 * time.Second
	WSLWarningDelay     = 20 * time.Second
	DeprecationDelay    = 10 * time.Second
	RetryDelay          = 10 * time.Second

	// Paths.
	DefaultDownloadURL = "https://download.docker.com"
	DefaultRepoFile    = "docker-ce.repo"

	// Docker paths.
	DockerBinPath1 = "/usr/bin/docker"
	DockerBinPath2 = "/usr/local/bin/docker"
	DockerBinPath3 = "/bin/docker"
	DockerSocket   = "/var/run/docker.sock"

	// Files.
	OSReleaseFile   = "/etc/os-release"
	ProcVersionFile = "/proc/version"
	DebianVersion   = "/etc/debian_version"

	// Package names.
	PkgDockerCE             = "docker-ce"
	PkgDockerCECLI          = "docker-ce-cli"
	PkgContainerd           = "containerd.io"
	PkgDockerCompose        = "docker-compose-plugin"
	PkgDockerBuildx         = "docker-buildx-plugin"
	PkgDockerScan           = "docker-scan-plugin"
	PkgDockerRootlessExtras = "docker-ce-rootless-extras"

	// Shell commands.
	ShellEcho = "echo"

	// Distro names.
	DistroUbuntu   = "ubuntu"
	DistroRaspbian = "raspbian"
	DistroOSMC     = "osmc"
	DistroDebian   = "debian"
)
