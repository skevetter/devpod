package dockerinstall

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type distro struct {
	id      string
	version string
}

func Install(stdout, stderr io.Writer) (string, error) {
	opts := &installOptions{
		channel:     getEnv("CHANNEL", "stable"),
		version:     strings.TrimPrefix(getEnv("VERSION", ""), "v"),
		downloadURL: getEnv("DOWNLOAD_URL", "https://download.docker.com"),
		repoFile:    getEnv("REPO_FILE", "docker-ce.repo"),
		dryRun:      getEnv("DRY_RUN", "") != "",
		stdout:      stdout,
		stderr:      stderr,
	}
	if err := doInstall(opts); err != nil {
		return "", fmt.Errorf("docker installation failed: %w", err)
	}
	
	commonPaths := []string{"/usr/bin/docker", "/usr/local/bin/docker", "/bin/docker"}
	for _, path := range commonPaths {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "docker", nil
}

type installOptions struct {
	channel     string
	version     string
	downloadURL string
	repoFile    string
	dryRun      bool
	stdout      io.Writer
	stderr      io.Writer
}

func doInstall(opts *installOptions) error {
	if runtime.GOOS == "darwin" {
		_, _ = fmt.Fprintln(opts.stderr, "")
		_, _ = fmt.Fprintln(opts.stderr, "ERROR: Unsupported operating system 'macOS'")
		_, _ = fmt.Fprintln(opts.stderr, "Please get Docker Desktop from https://www.docker.com/products/docker-desktop")
		fprintln(opts.stderr, "")
		return fmt.Errorf("unsupported OS")
	}

	if runtime.GOOS != "linux" {
		return fmt.Errorf("docker installation only supported on Linux")
	}

	if commandExists("docker") {
		fprintln(opts.stderr, "Warning: the \"docker\" command appears to already exist on this system.")
		fprintln(opts.stderr, "")
		fprintln(opts.stderr, "If you already have Docker installed, this script can cause trouble, which is")
		fprintln(opts.stderr, "why we're displaying this warning and provide the opportunity to cancel the")
		fprintln(opts.stderr, "installation.")
		fprintln(opts.stderr, "")
		fprintln(opts.stderr, "If you installed the current Docker package using this script and are using it")
		fprintln(opts.stderr, "again to update Docker, you can safely ignore this message.")
		fprintln(opts.stderr, "")
		fprintln(opts.stderr, "You may press Ctrl+C now to abort this script.")
		if !opts.dryRun {
			runWithSetX(opts, 20, func() { time.Sleep(20 * time.Second) })
		}
	}

	user := getUser()
	shC := "sh -c"
	if user != "root" {
		if commandExists("sudo") {
			shC = "sudo -E sh -c"
		} else if commandExists("su") {
			shC = "su -c"
		} else {
			fprintln(opts.stderr, "Error: this installer needs the ability to run commands as root.")
			fprintln(opts.stderr, "We are unable to find either \"sudo\" or \"su\" available to make this happen.")
			return fmt.Errorf("cannot run as root")
		}
	}

	if opts.dryRun {
		shC = "echo"
	}

	if isWSL() {
		fprintln(opts.stdout, "")
		fprintln(opts.stdout, "WSL DETECTED: We recommend using Docker Desktop for Windows.")
		fprintln(opts.stdout, "Please get Docker Desktop from https://www.docker.com/products/docker-desktop")
		fprintln(opts.stdout, "")
		fprintln(opts.stderr, "")
		fprintln(opts.stderr, "You may press Ctrl+C now to abort this script.")
		if !opts.dryRun {
			runWithSetX(opts, 20, func() { time.Sleep(20 * time.Second) })
		}
	}

	distro := getDistribution()
	distro = checkForked(distro)
	checkDeprecation(distro, opts)

	switch distro.id {
	case "ubuntu", "debian", "raspbian":
		return installDebian(distro, shC, opts)
	case "centos", "fedora", "rhel":
		return installRHEL(distro, shC, opts)
	case "sles":
		return installSLES(distro, shC, opts)
	default:
		if distro.id == "" {
			if runtime.GOOS == "darwin" {
				fprintln(opts.stderr, "")
				fprintln(opts.stderr, "ERROR: Unsupported operating system 'macOS'")
				fprintln(opts.stderr, "Please get Docker Desktop from https://www.docker.com/products/docker-desktop")
				fprintln(opts.stderr, "")
				return fmt.Errorf("unsupported OS")
			}
		}
		fprintln(opts.stderr, "")
		_, _ = fmt.Fprintf(opts.stderr, "ERROR: Unsupported distribution '%s'\n", distro.id)
		fprintln(opts.stderr, "")
		return fmt.Errorf("unsupported distribution")
	}
}

func commandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

func isWSL() bool {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	v := strings.ToLower(string(data))
	return strings.Contains(v, "microsoft") || strings.Contains(v, "wsl")
}

func getUser() string {
	if user := os.Getenv("USER"); user != "" {
		return user
	}
	cmd := exec.Command("id", "-un")
	out, _ := cmd.Output()
	return strings.TrimSpace(string(out))
}

func getDistribution() *distro {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return &distro{}
	}
	defer func() { _ = f.Close() }()

	distro := &distro{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "ID=") {
			distro.id = strings.Trim(strings.TrimPrefix(line, "ID="), "\"")
		} else if strings.HasPrefix(line, "VERSION_CODENAME=") {
			distro.version = strings.Trim(strings.TrimPrefix(line, "VERSION_CODENAME="), "\"")
		} else if strings.HasPrefix(line, "VERSION_ID=") && distro.version == "" {
			distro.version = strings.Trim(strings.TrimPrefix(line, "VERSION_ID="), "\"")
		}
	}
	distro.id = strings.ToLower(distro.id)
	return distro
}

func checkForked(distro *distro) *distro {
	if !commandExists("lsb_release") {
		if data, err := os.ReadFile("/etc/debian_version"); err == nil && distro.id != "ubuntu" && distro.id != "raspbian" {
			if distro.id == "osmc" {
				distro.id = "raspbian"
			} else {
				distro.id = "debian"
			}
			version := strings.TrimSpace(string(data))
			version = strings.Split(version, "/")[0]
			version = strings.Split(version, ".")[0]
			switch version {
			case "11":
				distro.version = "bullseye"
			case "10":
				distro.version = "buster"
			case "9":
				distro.version = "stretch"
			case "8":
				distro.version = "jessie"
			}
		}
		return distro
	}

	cmd := exec.Command("lsb_release", "-a", "-u")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return distro
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.ToLower(line)
		if strings.Contains(line, "distributor id:") {
			parts := strings.Split(line, ":")
			if len(parts) == 2 {
				distro.id = strings.TrimSpace(parts[1])
			}
		} else if strings.Contains(line, "codename:") {
			parts := strings.Split(line, ":")
			if len(parts) == 2 {
				distro.version = strings.TrimSpace(parts[1])
			}
		}
	}

	return distro
}

func checkDeprecation(distro *distro, opts *installOptions) {
	deprecated := false
	key := distro.id + "." + distro.version
	switch key {
	case "debian.stretch", "debian.jessie", "raspbian.stretch", "raspbian.jessie", "ubuntu.xenial", "ubuntu.trusty":
		deprecated = true
	}

	if distro.id == "fedora" {
		if ver, err := strconv.Atoi(distro.version); err == nil && ver < 33 {
			deprecated = true
		}
	}

	if deprecated {
		fprintln(opts.stdout, "")
		fprintln(opts.stdout, "\033[91;1mDEPRECATION WARNING\033[0m")
		_, _ = fmt.Fprintf(opts.stdout, "    This Linux distribution (\033[1m%s %s\033[0m) reached end-of-life and is no longer supported by this script.\n", distro.id, distro.version)
		fprintln(opts.stdout, "    No updates or security fixes will be released for this distribution, and users are recommended")
		_, _ = fmt.Fprintf(opts.stdout, "    to upgrade to a currently maintained version of %s.\n", distro.id)
		fprintln(opts.stdout, "")
		fprintln(opts.stdout, "Press \033[1mCtrl+C\033[0m now to abort this script, or wait for the installation to continue.")
		fprintln(opts.stdout, "")
		time.Sleep(10 * time.Second)
	}
}

func installDebian(distro *distro, shC string, opts *installOptions) error {
	preReqs := "apt-transport-https ca-certificates curl"
	if !commandExists("gpg") {
		preReqs += " gnupg"
	}

	aptRepo := fmt.Sprintf("deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] %s/linux/%s %s %s",
		opts.downloadURL, distro.id, distro.version, opts.channel)

	cmds := []string{
		"apt-get update -qq >/dev/null",
		fmt.Sprintf("DEBIAN_FRONTEND=noninteractive apt-get install -y -qq %s >/dev/null", preReqs),
		"mkdir -p /etc/apt/keyrings && chmod -R 0755 /etc/apt/keyrings",
		fmt.Sprintf("curl -fsSL \"%s/linux/%s/gpg\" | gpg --dearmor --yes -o /etc/apt/keyrings/docker.gpg", opts.downloadURL, distro.id),
		"chmod a+r /etc/apt/keyrings/docker.gpg",
		fmt.Sprintf("echo \"%s\" > /etc/apt/sources.list.d/docker.list", aptRepo),
		"apt-get update -qq >/dev/null",
	}

	if err := runCommands(shC, opts, cmds); err != nil {
		return err
	}

	pkgVersion := ""
	cliPkgVersion := ""
	if opts.version != "" {
		if opts.dryRun {
			fprintln(opts.stdout, "# WARNING: VERSION pinning is not supported in DRY_RUN")
		} else {
			pkgPattern := strings.ReplaceAll(opts.version, "-ce-", "~ce~.*")
			pkgPattern = strings.ReplaceAll(pkgPattern, "-", ".*")
			searchCmd := fmt.Sprintf("apt-cache madison 'docker-ce' | grep '%s' | head -1 | awk '{$1=$1};1' | cut -d' ' -f 3", pkgPattern)
			
			fprintln(opts.stdout, "INFO: Searching repository for VERSION '"+opts.version+"'")
			fprintln(opts.stdout, "INFO: "+searchCmd)
			
			cmd := exec.Command("sh", "-c", searchCmd)
			out, err := cmd.Output()
			if err != nil || len(out) == 0 {
				fprintln(opts.stderr, "")
				fprintf(opts.stderr, "ERROR: '%s' not found amongst apt-cache madison results\n", opts.version)
				fprintln(opts.stderr, "")
				return fmt.Errorf("version not found")
			}
			pkgVersion = "=" + strings.TrimSpace(string(out))

			if versionGte(opts.version, "18.09") {
				searchCmd = fmt.Sprintf("apt-cache madison 'docker-ce-cli' | grep '%s' | head -1 | awk '{$1=$1};1' | cut -d' ' -f 3", pkgPattern)
				fprintln(opts.stdout, "INFO: "+searchCmd)
				cmd = exec.Command("sh", "-c", searchCmd)
				out, _ = cmd.Output()
				cliPkgVersion = "=" + strings.TrimSpace(string(out))
			}
		}
	}

	pkgs := buildPackageList(opts.version, strings.TrimSuffix(pkgVersion, "="), strings.TrimSuffix(cliPkgVersion, "="))

	installCmd := fmt.Sprintf("DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends %s >/dev/null", pkgs)
	if err := runShellCmd(shC, installCmd, opts); err != nil {
		return err
	}

	echoDockerAsNonroot(opts)
	return nil
}

func installRHEL(distro *distro, shC string, opts *installOptions) error {
	if distro.id == "rhel" && runtime.GOARCH != "s390x" {
		fprintln(opts.stderr, "Packages for RHEL are currently only available for s390x.")
		return fmt.Errorf("unsupported arch")
	}

	pkgMgr := "yum"
	configMgr := "yum-config-manager"
	enableFlag := "--enable"
	disableFlag := "--disable"
	preReqs := "yum-utils"
	pkgSuffix := "el"

	if distro.id == "fedora" {
		pkgMgr = "dnf"
		configMgr = "dnf config-manager"
		enableFlag = "--set-enabled"
		disableFlag = "--set-disabled"
		preReqs = "dnf-plugins-core"
		pkgSuffix = "fc" + distro.version
	}

	repoURL := fmt.Sprintf("%s/linux/%s/%s", opts.downloadURL, distro.id, opts.repoFile)
	cmds := []string{
		fmt.Sprintf("%s install -y -q %s", pkgMgr, preReqs),
		fmt.Sprintf("%s --add-repo %s", configMgr, repoURL),
	}

	if opts.channel != "stable" {
		cmds = append(cmds, fmt.Sprintf("%s %s docker-ce-*", configMgr, disableFlag))
		cmds = append(cmds, fmt.Sprintf("%s %s docker-ce-%s", configMgr, enableFlag, opts.channel))
	}

	cmds = append(cmds, fmt.Sprintf("%s makecache", pkgMgr))

	if err := runCommands(shC, opts, cmds); err != nil {
		return err
	}

	pkgVersion := ""
	cliPkgVersion := ""
	if opts.version != "" {
		if opts.dryRun {
			fprintln(opts.stdout, "# WARNING: VERSION pinning is not supported in DRY_RUN")
		} else {
			pkgPattern := strings.ReplaceAll(opts.version, "-ce-", "\\\\.ce.*")
			pkgPattern = strings.ReplaceAll(pkgPattern, "-", ".*") + ".*" + pkgSuffix
			searchCmd := fmt.Sprintf("%s list --showduplicates 'docker-ce' | grep '%s' | tail -1 | awk '{print $2}'", pkgMgr, pkgPattern)
			
			fprintln(opts.stdout, "INFO: Searching repository for VERSION '"+opts.version+"'")
			fprintln(opts.stdout, "INFO: "+searchCmd)
			
			cmd := exec.Command("sh", "-c", searchCmd)
			out, err := cmd.Output()
			if err != nil || len(out) == 0 {
				fprintln(opts.stderr, "")
				fprintf(opts.stderr, "ERROR: '%s' not found amongst %s list results\n", opts.version, pkgMgr)
				fprintln(opts.stderr, "")
				return fmt.Errorf("version not found")
			}
			version := strings.TrimSpace(string(out))
			parts := strings.Split(version, ":")
			if len(parts) == 2 {
				pkgVersion = "-" + parts[1]
			} else {
				pkgVersion = "-" + version
			}

			if versionGte(opts.version, "18.09") {
				searchCmd = fmt.Sprintf("%s list --showduplicates 'docker-ce-cli' | grep '%s' | tail -1 | awk '{print $2}'", pkgMgr, pkgPattern)
				cmd = exec.Command("sh", "-c", searchCmd)
				out, _ = cmd.Output()
				if len(out) > 0 {
					version = strings.TrimSpace(string(out))
					parts = strings.Split(version, ":")
					if len(parts) == 2 {
						cliPkgVersion = "-" + parts[1]
					}
				}
			}
		}
	}

	var extraPkgs []string
	if runtime.GOARCH == "amd64" || runtime.GOARCH == "x86_64" {
		extraPkgs = append(extraPkgs, "docker-scan-plugin")
	}
	extraPkgs = append(extraPkgs, "docker-ce-rootless-extras"+pkgVersion)
	
	pkgs := buildPackageList(opts.version, pkgVersion, cliPkgVersion, extraPkgs...)

	installCmd := fmt.Sprintf("%s install -y -q %s", pkgMgr, pkgs)
	if err := runShellCmd(shC, installCmd, opts); err != nil {
		return err
	}

	echoDockerAsNonroot(opts)
	return nil
}

func installSLES(distro *distro, shC string, opts *installOptions) error {
	if runtime.GOARCH != "s390x" {
		fprintln(opts.stderr, "Packages for SLES are currently only available for s390x")
		return fmt.Errorf("unsupported arch")
	}

	slesVersion := "SLE_15_SP3"
	if distro.version == "15.3" {
		slesVersion = "SLE_15_SP3"
	} else {
		parts := strings.Split(distro.version, ".")
		if len(parts) >= 2 {
			slesVersion = "15." + parts[1]
		}
	}

	opensusRepo := fmt.Sprintf("https://download.opensuse.org/repositories/security:SELinux/%s/security:SELinux.repo", slesVersion)
	repoURL := fmt.Sprintf("%s/linux/sles/%s", opts.downloadURL, opts.repoFile)

	cmds := []string{
		"zypper install -y ca-certificates curl libseccomp2 awk",
		fmt.Sprintf("zypper addrepo %s", repoURL),
	}

	if err := runCommands(shC, opts, cmds); err != nil {
		return err
	}

	if !opts.dryRun {
		fprintln(opts.stderr, "WARNING!!")
		fprintf(opts.stderr, "openSUSE repository (%s) will be enabled now.\n", opensusRepo)
		fprintln(opts.stderr, "Do you wish to continue?")
		fprintln(opts.stderr, "You may press Ctrl+C now to abort this script.")
		runWithSetX(opts, 30, func() { time.Sleep(30 * time.Second) })
	}

	moreCmds := []string{
		fmt.Sprintf("zypper addrepo %s", opensusRepo),
		"zypper --gpg-auto-import-keys refresh",
		"zypper lr -d",
	}

	if err := runCommands(shC, opts, moreCmds); err != nil {
		return err
	}

	pkgVersion := ""
	cliPkgVersion := ""
	rootlessPkgVersion := ""
	if opts.version != "" {
		if opts.dryRun {
			fprintln(opts.stdout, "# WARNING: VERSION pinning is not supported in DRY_RUN")
		} else {
			pkgPattern := strings.ReplaceAll(opts.version, "-ce-", "\\\\.ce.*")
			pkgPattern = strings.ReplaceAll(pkgPattern, "-", ".*")
			searchCmd := fmt.Sprintf("zypper search -s --match-exact 'docker-ce' | grep '%s' | tail -1 | awk '{print $6}'", pkgPattern)
			
			fprintln(opts.stdout, "INFO: Searching repository for VERSION '"+opts.version+"'")
			fprintln(opts.stdout, "INFO: "+searchCmd)
			
			cmd := exec.Command("sh", "-c", searchCmd)
			out, err := cmd.Output()
			if err != nil || len(out) == 0 {
				fprintln(opts.stderr, "")
				fprintf(opts.stderr, "ERROR: '%s' not found amongst zypper list results\n", opts.version)
				fprintln(opts.stderr, "")
				return fmt.Errorf("version not found")
			}
			pkgVersion = "-" + strings.TrimSpace(string(out))

			searchCmd = fmt.Sprintf("zypper search -s --match-exact 'docker-ce-cli' | grep '%s' | tail -1 | awk '{print $6}'", pkgPattern)
			cmd = exec.Command("sh", "-c", searchCmd)
			out, _ = cmd.Output()
			if len(out) > 0 {
				cliPkgVersion = "-" + strings.TrimSpace(string(out))
			}

			searchCmd = fmt.Sprintf("zypper search -s --match-exact 'docker-ce-rootless-extras' | grep '%s' | tail -1 | awk '{print $6}'", pkgPattern)
			cmd = exec.Command("sh", "-c", searchCmd)
			out, _ = cmd.Output()
			if len(out) > 0 {
				rootlessPkgVersion = "-" + strings.TrimSpace(string(out))
			}
		}
	}

	pkgs := buildPackageList(opts.version, pkgVersion, cliPkgVersion, "docker-ce-rootless-extras"+rootlessPkgVersion)

	installCmd := fmt.Sprintf("zypper -q install -y %s", pkgs)
	if err := runShellCmd(shC, installCmd, opts); err != nil {
		return err
	}

	echoDockerAsNonroot(opts)
	return nil
}

func runShellCmd(shC, cmdStr string, opts *installOptions) error {
	if !opts.dryRun {
		fprintf(opts.stderr, "+ %s\n", cmdStr)
	}

	var cmd *exec.Cmd
	if strings.HasPrefix(shC, "sudo") {
		cmd = exec.Command("sudo", "-E", "sh", "-c", cmdStr)
	} else if strings.HasPrefix(shC, "su") {
		cmd = exec.Command("su", "-c", cmdStr)
	} else if shC == "echo" {
		fprintln(opts.stdout, cmdStr)
		return nil
	} else {
		cmd = exec.Command("sh", "-c", cmdStr)
	}

	cmd.Stdout = opts.stdout
	cmd.Stderr = opts.stderr
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	return cmd.Run()
}

func runWithSetX(opts *installOptions, duration int, fn func()) {
	fprintf(opts.stderr, "+ sleep %d\n", duration)
	fn()
}

func versionGte(version, target string) bool {
	if version == "" {
		return true
	}

	vParts := strings.Split(version, ".")
	tParts := strings.Split(target, ".")

	for i := 0; i < len(tParts) && i < len(vParts); i++ {
		vNum, _ := strconv.Atoi(strings.Split(vParts[i], "-")[0])
		tNum, _ := strconv.Atoi(tParts[i])

		if vNum > tNum {
			return true
		}
		if vNum < tNum {
			return false
		}
	}

	return true
}

func echoDockerAsNonroot(opts *installOptions) {
	if opts.dryRun {
		return
	}

	if commandExists("docker") {
		if _, err := os.Stat("/var/run/docker.sock"); err == nil {
			fprintf(opts.stderr, "+ docker version\n")
			cmd := exec.Command("docker", "version")
			cmd.Stdout = opts.stdout
			cmd.Stderr = opts.stdout
			_ = cmd.Run()
		}
	}

	fprintln(opts.stdout, "")
	fprintln(opts.stdout, "================================================================================")
	fprintln(opts.stdout, "")
	if versionGte(opts.version, "20.10") {
		fprintln(opts.stdout, "To run Docker as a non-privileged user, consider setting up the")
		fprintln(opts.stdout, "Docker daemon in rootless mode for your user:")
		fprintln(opts.stdout, "")
		fprintln(opts.stdout, "    dockerd-rootless-setuptool.sh install")
		fprintln(opts.stdout, "")
		fprintln(opts.stdout, "Visit https://docs.docker.com/go/rootless/ to learn about rootless mode.")
		fprintln(opts.stdout, "")
	}
	fprintln(opts.stdout, "")
	fprintln(opts.stdout, "To run the Docker daemon as a fully privileged service, but granting non-root")
	fprintln(opts.stdout, "users access, refer to https://docs.docker.com/go/daemon-access/")
	fprintln(opts.stdout, "")
	fprintln(opts.stdout, "WARNING: Access to the remote API on a privileged Docker daemon is equivalent")
	fprintln(opts.stdout, "         to root access on the host. Refer to the 'Docker daemon attack surface'")
	fprintln(opts.stdout, "         documentation for details: https://docs.docker.com/go/attack-surface/")
	fprintln(opts.stdout, "")
	fprintln(opts.stdout, "================================================================================")
	fprintln(opts.stdout, "")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// Helper functions to ignore fmt errors (matching shell script behavior)
func fprintln(w io.Writer, a ...interface{}) {
	_, _ = fmt.Fprintln(w, a...)
}

func fprintf(w io.Writer, format string, a ...interface{}) {
	_, _ = fmt.Fprintf(w, format, a...)
}

func buildPackageList(version, pkgVersion, cliPkgVersion string, extraPkgs ...string) string {
	pkgs := "docker-ce" + pkgVersion
	
	if versionGte(version, "18.09") {
		if cliPkgVersion != "" {
			pkgs += " docker-ce-cli" + cliPkgVersion + " containerd.io"
		} else {
			pkgs += " docker-ce-cli containerd.io"
		}
	}
	
	if versionGte(version, "20.10") {
		pkgs += " docker-compose-plugin"
		for _, extra := range extraPkgs {
			pkgs += " " + extra
		}
	}
	
	if versionGte(version, "23.0") {
		pkgs += " docker-buildx-plugin"
	}
	
	return pkgs
}

func runCommands(shC string, opts *installOptions, cmds []string) error {
	for _, cmd := range cmds {
		if err := runShellCmd(shC, cmd, opts); err != nil {
			return err
		}
	}
	return nil
}
