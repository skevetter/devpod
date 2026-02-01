package dockerinstall

import (
	"strconv"
	"strings"
)

func BuildPackageList(version, pkgVersion, cliPkgVersion string, extraPkgs ...string) string {
	var pkgs []string

	// Docker CE
	pkgs = append(pkgs, PkgDockerCE+pkgVersion)

	// CLI and containerd (18.09+)
	if versionGte(version, "18.09") {
		if cliPkgVersion != "" {
			pkgs = append(pkgs, PkgDockerCECLI+cliPkgVersion, PkgContainerd)
		} else {
			pkgs = append(pkgs, PkgDockerCECLI, PkgContainerd)
		}
	}

	// Compose plugin (20.10+)
	if versionGte(version, "20.10") {
		pkgs = append(pkgs, PkgDockerCompose)
		pkgs = append(pkgs, extraPkgs...)
	}

	// Buildx plugin (23.0+)
	if versionGte(version, "23.0") {
		pkgs = append(pkgs, PkgDockerBuildx)
	}

	return strings.Join(pkgs, " ")
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
