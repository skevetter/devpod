package config

// Environment variable constants used throughout the application.
// All constants follow the EnvXxx naming convention.
const (
	// EnvBinaryPath is set to the path of the DevPod binary.
	EnvBinaryPath = "DEVPOD"

	// EnvHome overrides the default DevPod home directory.
	EnvHome = "DEVPOD_HOME"

	// EnvConfig overrides the default config file path.
	EnvConfig = "DEVPOD_CONFIG"

	// EnvUI indicates the desktop UI is active.
	EnvUI = "DEVPOD_UI"

	// EnvDebug enables debug logging.
	EnvDebug = "DEVPOD_DEBUG"

	// EnvDisableTelemetry disables telemetry collection.
	EnvDisableTelemetry = "DEVPOD_DISABLE_TELEMETRY"

	// EnvAgentURL overrides the agent download URL.
	EnvAgentURL = "DEVPOD_AGENT_URL"

	// EnvAgentPreferDownload forces agent binary download even if a local copy exists.
	EnvAgentPreferDownload = "DEVPOD_AGENT_PREFER_DOWNLOAD"

	// EnvOS is set to the host operating system (runtime.GOOS).
	EnvOS = "DEVPOD_OS"

	// EnvArch is set to the host architecture (runtime.GOARCH).
	EnvArch = "DEVPOD_ARCH"

	// EnvLogLevel is set to the current log level.
	EnvLogLevel = "DEVPOD_LOG_LEVEL"

	// EnvWorkspaceID is the current workspace identifier.
	EnvWorkspaceID = "DEVPOD_WORKSPACE_ID"

	// EnvWorkspaceUID is the current workspace unique identifier.
	EnvWorkspaceUID = "DEVPOD_WORKSPACE_UID"

	// EnvWorkspaceDaemonConfig holds the workspace daemon configuration.
	EnvWorkspaceDaemonConfig = "DEVPOD_WORKSPACE_DAEMON_CONFIG"

	// EnvWorkspaceCredentialsPort is the workspace credentials server port.
	EnvWorkspaceCredentialsPort = "DEVPOD_WORKSPACE_CREDENTIALS_PORT" // #nosec G101

	// EnvCredentialsServerPort is the credentials server port on the host side.
	EnvCredentialsServerPort = "DEVPOD_CREDENTIALS_SERVER_PORT" // #nosec G101

	// EnvGitHelperPort is the git credential helper forwarding port.
	EnvGitHelperPort = "DEVPOD_GIT_HELPER_PORT"

	// EnvCraneName overrides the crane binary name.
	EnvCraneName = "DEVPOD_CRANE_NAME"

	// EnvPlatformOptions holds serialized platform options.
	EnvPlatformOptions = "DEVPOD_PLATFORM_OPTIONS"

	// EnvFlagsUp holds extra flags for the up command.
	EnvFlagsUp = "DEVPOD_FLAGS_UP"

	// EnvFlagsSSH holds extra flags for the ssh command.
	EnvFlagsSSH = "DEVPOD_FLAGS_SSH"

	// EnvFlagsDelete holds extra flags for the delete command.
	EnvFlagsDelete = "DEVPOD_FLAGS_DELETE"

	// EnvFlagsStatus holds extra flags for the status command.
	EnvFlagsStatus = "DEVPOD_FLAGS_STATUS"

	// EnvSubdomain is the subdomain configuration for DevPod Pro.
	EnvSubdomain = "DEVPOD_SUBDOMAIN"

	// EnvPrefix is the base prefix for all DevPod environment variables.
	EnvPrefix = "DEVPOD_"

	// EnvIDEPrefix is the prefix for IDE-specific option env vars (append IDE name + "_").
	EnvIDEPrefix = EnvPrefix + "IDE_"

	// EnvProviderPrefix is the prefix for provider-specific option env vars (append provider name + "_").
	EnvProviderPrefix = EnvPrefix + "PROVIDER_"
)
