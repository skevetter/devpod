package dockerinstall

import (
	"fmt"
	"io"
	"strconv"
	"time"
)

type Validator struct {
	opts *InstallOptions
}

func NewValidator(opts *InstallOptions) *Validator {
	return &Validator{opts: opts}
}

func (v *Validator) ValidateOS(os string) error {
	if os == "darwin" {
		fprintln(v.opts.stderr, `
ERROR: Unsupported operating system 'macOS'
Please get Docker Desktop from https://www.docker.com/products/docker-desktop`)
		return fmt.Errorf("unsupported OS")
	}

	if os != "linux" {
		return fmt.Errorf("docker installation only supported on Linux")
	}

	return nil
}

func (v *Validator) CheckExistingDocker() {
	if !commandExists("docker") {
		return
	}

	fprintln(v.opts.stderr, `Warning: the "docker" command appears to already exist on this system.

If you already have Docker installed, this script can cause trouble, which is
why we're displaying this warning and provide the opportunity to cancel the
installation.

If you installed the current Docker package using this script and are using it
again to update Docker, you can safely ignore this message.

You may press Ctrl+C now to abort this script.`)
	v.sleep(ExistingDockerDelay)
}

func (v *Validator) CheckWSL(isWSL bool) {
	if !isWSL {
		return
	}

	fprintln(v.opts.stdout, `
WSL DETECTED: We recommend using Docker Desktop for Windows.
Please get Docker Desktop from https://www.docker.com/products/docker-desktop`)
	fprintln(v.opts.stderr, `
You may press Ctrl+C now to abort this script.`)
	v.sleep(WSLWarningDelay)
}

func (v *Validator) CheckDeprecation(distro *Distro) {
	deprecated := false
	key := distro.ID + "." + distro.Version
	switch key {
	case "debian.stretch", "debian.jessie", "raspbian.stretch", "raspbian.jessie", "ubuntu.xenial", "ubuntu.trusty":
		deprecated = true
	}

	if distro.ID == "fedora" {
		if ver, err := strconv.Atoi(distro.Version); err == nil && ver < 33 {
			deprecated = true
		}
	}

	if !deprecated {
		return
	}

	fprintf(v.opts.stdout, `
DEPRECATION WARNING
    This Linux distribution (%s %s) reached end-of-life and is no longer supported by this script.
    No updates or security fixes will be released for this distribution, and users are recommended
    to upgrade to a currently maintained version of %s.

Press Ctrl+C now to abort this script, or wait for the installation to continue.`,
		distro.ID, distro.Version, distro.ID)
	v.sleep(DeprecationDelay)
}

func (v *Validator) ValidateDistro(distro *Distro) error {
	if distro.ID == "" {
		fprintln(v.opts.stderr, `
ERROR: Unable to detect distribution`)
		return fmt.Errorf("unknown distribution")
	}

	supported := map[string]bool{
		"ubuntu": true, "debian": true, "raspbian": true,
		"centos": true, "fedora": true, "rhel": true,
		"sles": true,
	}

	if !supported[distro.ID] {
		fprintf(v.opts.stderr, `
ERROR: Unsupported distribution '%s'`, distro.ID)
		return fmt.Errorf("unsupported distribution")
	}

	return nil
}

func (v *Validator) sleep(duration time.Duration) {
	if v.opts.dryRun {
		return
	}
	time.Sleep(duration)
}

func fprintln(w io.Writer, a ...any) {
	_, _ = fmt.Fprintln(w, a...)
}

func fprintf(w io.Writer, format string, a ...any) {
	_, _ = fmt.Fprintf(w, format, a...)
}
