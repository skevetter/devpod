package dockerinstall

import "io"

type InstallOptions struct {
	channel     string
	version     string
	downloadURL string
	repoFile    string
	dryRun      bool
	stdout      io.Writer
	stderr      io.Writer
}

type Installer interface {
	Install(shC string) error
}
