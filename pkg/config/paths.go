package config

const (
	// IgnoreFileName is the name of the devpod ignore file.
	IgnoreFileName = "." + BinaryName + "ignore"

	// SSHSignatureHelperPath is the path to the SSH signature helper script.
	SSHSignatureHelperPath = "/usr/local/bin/" + BinaryName + "-ssh-signature"

	// SSHSignatureHelperName is the name used in git config for the SSH signature program.
	SSHSignatureHelperName = BinaryName + "-ssh-signature"

	// DockerCredentialHelperName is the docker credential helper binary name.
	DockerCredentialHelperName = "docker-credential-" + BinaryName

	// DevContainerResultPath is where devcontainer results are written.
	DevContainerResultPath = "/var/run/" + BinaryName + "/result.json"
)
