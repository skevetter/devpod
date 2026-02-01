package dockerinstall

import (
	"fmt"
	"os/exec"
	"strings"
)

type DebianInstaller struct {
	distro   *Distro
	opts     *InstallOptions
	executor *Executor
}

func NewDebianInstaller(distro *Distro, opts *InstallOptions) *DebianInstaller {
	return &DebianInstaller{
		distro:   distro,
		opts:     opts,
		executor: NewExecutor(opts),
	}
}

func (i *DebianInstaller) Install(shC string) error {
	if err := i.setupRepo(shC); err != nil {
		return err
	}

	pkgVersion, cliPkgVersion := i.findVersions()
	pkgs := BuildPackageList(i.opts.version, pkgVersion, cliPkgVersion)

	installCmd := fmt.Sprintf(
		"DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends %s >/dev/null",
		pkgs,
	)
	if err := i.executor.RunWithRetry(shC, installCmd, DefaultTimeout); err != nil {
		return err
	}

	echoDockerAsNonroot(i.opts)
	return nil
}

func (i *DebianInstaller) setupRepo(shC string) error {
	preReqs := "apt-transport-https ca-certificates curl"
	if !commandExists("gpg") {
		preReqs += " gnupg"
	}

	aptRepo := fmt.Sprintf(
		"deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] %s/linux/%s %s %s",
		i.opts.downloadURL, i.distro.ID, i.distro.Version, i.opts.channel,
	)

	cmds := []string{
		"apt-get update -qq >/dev/null",
		fmt.Sprintf("DEBIAN_FRONTEND=noninteractive apt-get install -y -qq %s >/dev/null", preReqs),
		"mkdir -p /etc/apt/keyrings && chmod -R 0755 /etc/apt/keyrings",
		fmt.Sprintf(
			"curl -fsSL \"%s/linux/%s/gpg\" | gpg --dearmor --yes -o /etc/apt/keyrings/docker.gpg",
			i.opts.downloadURL, i.distro.ID,
		),
		"chmod a+r /etc/apt/keyrings/docker.gpg",
		fmt.Sprintf("echo \"%s\" > /etc/apt/sources.list.d/docker.list", aptRepo),
		"apt-get update -qq >/dev/null",
	}

	return i.executor.RunCommandsWithRetry(shC, cmds, DefaultTimeout)
}

func (i *DebianInstaller) findVersions() (string, string) {
	if i.opts.version == "" || i.opts.dryRun {
		if i.opts.dryRun {
			fprintln(i.opts.stdout, "# WARNING: VERSION pinning is not supported in DRY_RUN")
		}
		return "", ""
	}

	pkgPattern := strings.ReplaceAll(i.opts.version, "-ce-", "~ce~.*")
	pkgPattern = strings.ReplaceAll(pkgPattern, "-", ".*")
	searchCmd := fmt.Sprintf(
		"apt-cache madison 'docker-ce' | grep '%s' | head -1 | awk '{$1=$1};1' | cut -d' ' -f 3",
		pkgPattern,
	)

	fprintln(i.opts.stdout, "INFO: Searching repository for VERSION '"+i.opts.version+"'")
	fprintln(i.opts.stdout, "INFO: "+searchCmd)

	//nolint:gosec // Intentional shell command for apt repository search
	cmd := exec.Command("sh", "-c", searchCmd)
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		fprintf(i.opts.stderr, `
ERROR: '%s' not found amongst apt-cache madison results`, i.opts.version)
		return "", ""
	}
	pkgVersion := "=" + strings.TrimSpace(string(out))

	cliPkgVersion := ""
	if versionGte(i.opts.version, "18.09") {
		searchCmd = fmt.Sprintf(
			"apt-cache madison 'docker-ce-cli' | grep '%s' | head -1 | awk '{$1=$1};1' | cut -d' ' -f 3",
			pkgPattern,
		)
		fprintln(i.opts.stdout, "INFO: "+searchCmd)
		//nolint:gosec // Intentional shell command for apt repository search
		cmd = exec.Command("sh", "-c", searchCmd)
		out, _ = cmd.Output()
		cliPkgVersion = "=" + strings.TrimSpace(string(out))
	}

	return pkgVersion, cliPkgVersion
}
