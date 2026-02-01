package dockerinstall

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Distro represents a Linux distribution.
// ID is the distribution identifier (e.g., "ubuntu", "debian", "fedora").
// Version should be a codename (e.g., "jammy", "bookworm") for Debian-based distros,
// or a version number (e.g., "39") for others like Fedora.
type Distro struct {
	ID      string
	Version string
}

// HasCodename returns true if Version is a valid codename (non-numeric).
func (d *Distro) HasCodename() bool {
	return d.Version != "" && !isNumericVersion(d.Version)
}

type Detector struct{}

func NewDetector() *Detector {
	return &Detector{}
}

func (d *Detector) DetectOS() string {
	return runtime.GOOS
}

func (d *Detector) DetectDistro() *Distro {
	f, err := os.Open(OSReleaseFile)
	if err != nil {
		return &Distro{}
	}
	defer func() { _ = f.Close() }()

	distro := d.parseOSRelease(f)
	distro.ID = strings.ToLower(distro.ID)
	return distro
}

func (d *Detector) CheckForked(distro *Distro) *Distro {
	if !commandExists("lsb_release") {
		return d.checkDebianFork(distro)
	}
	return d.checkLSBRelease(distro)
}

func (d *Detector) IsWSL() bool {
	data, err := os.ReadFile(ProcVersionFile)
	if err != nil {
		return false
	}
	v := strings.ToLower(string(data))
	return strings.Contains(v, "microsoft") || strings.Contains(v, "wsl")
}

func (d *Detector) parseOSRelease(r io.Reader) *Distro {
	distro := &Distro{}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if after, ok := strings.CutPrefix(line, "ID="); ok {
			distro.ID = strings.Trim(after, "\"")
		} else if after, ok := strings.CutPrefix(line, "VERSION_CODENAME="); ok {
			distro.Version = strings.Trim(after, "\"")
		} else if strings.HasPrefix(line, "VERSION_ID=") && distro.Version == "" {
			distro.Version = strings.Trim(strings.TrimPrefix(line, "VERSION_ID="), "\"")
		}
	}
	if err := scanner.Err(); err != nil {
		return &Distro{}
	}
	return distro
}

func (d *Detector) checkDebianFork(distro *Distro) *Distro {
	data, err := os.ReadFile(DebianVersion)
	if err != nil || distro.ID == DistroUbuntu || distro.ID == DistroRaspbian {
		return distro
	}

	distro.ID = d.mapDebianID(distro.ID)
	distro.Version = d.mapDebianVersion(data)
	return distro
}

func (d *Detector) mapDebianID(id string) string {
	if id == DistroOSMC {
		return DistroRaspbian
	}
	return DistroDebian
}

func (d *Detector) mapDebianVersion(data []byte) string {
	version := strings.TrimSpace(string(data))
	version = strings.Split(version, "/")[0]
	version = strings.Split(version, ".")[0]

	versionMap := map[string]string{
		"13": "trixie",
		"12": "bookworm",
		"11": "bullseye",
		"10": "buster",
		"9":  "stretch",
		"8":  "jessie",
	}

	if mapped, ok := versionMap[version]; ok {
		return mapped
	}
	return ""
}

func (d *Detector) checkLSBRelease(distro *Distro) *Distro {
	cmd := exec.Command("lsb_release", "-a", "-u")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return distro
	}

	lines := strings.SplitSeq(string(output), "\n")
	for line := range lines {
		line = strings.ToLower(line)
		if strings.Contains(line, "distributor id:") {
			parts := strings.Split(line, ":")
			if len(parts) == 2 {
				distro.ID = strings.TrimSpace(parts[1])
			}
		} else if strings.Contains(line, "codename:") {
			parts := strings.Split(line, ":")
			if len(parts) == 2 {
				distro.Version = strings.TrimSpace(parts[1])
			}
		}
	}

	return distro
}

func commandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

// isNumericVersion returns true if the version string consists only of digits and dots.
func isNumericVersion(version string) bool {
	for _, ch := range version {
		if ch != '.' && (ch < '0' || ch > '9') {
			return false
		}
	}
	return len(version) > 0 && version[0] >= '0' && version[0] <= '9'
}
