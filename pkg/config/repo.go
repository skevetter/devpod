package config

const (
	RepoOwner         = "skevetter"
	RepoName          = "devpod"
	RepoSlug          = RepoOwner + "/" + RepoName
	GitHubRepoURL     = "https://github.com/" + RepoSlug
	GitHubReleasesURL = GitHubRepoURL + "/releases"
	GitHubAPIUserURL  = "https://api.github.com/users/" + RepoOwner
	ProviderPrefix    = RepoName + "-provider-"

	// ProReleaseName is the Helm release / product name for DevPod Pro.
	ProReleaseName = RepoName + "-pro"

	// BinaryName is the CLI binary base name used in downloads and SSH host suffixes.
	BinaryName = RepoName

	// SSHHostSuffix is appended to workspace IDs for SSH config host entries.
	SSHHostSuffix = "." + BinaryName

	// WebsiteBaseURL is the project website used for asset URLs.
	WebsiteBaseURL = "https://" + RepoName + ".sh"

	// WebsiteAssetsURL is the base URL for icon/image assets.
	WebsiteAssetsURL = WebsiteBaseURL + "/assets"
)
